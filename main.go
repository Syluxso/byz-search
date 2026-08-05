package main

import (
	"context"
	"log"
	"net/http"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/google/uuid"
)

const (
	defaultMaxSize     = 50
	defaultSize        = 20
	defaultDocMaxChars = 200_000
)

func main() {
	port := env("PORT", "8099")
	bind := env("BIND", "0.0.0.0")
	jwksURL := env("IAM_JWKS_URL", "https://iam.byzantineapp.dev/.well-known/jwks.json")
	solrURL := env("SOLR_URL", "http://127.0.0.1:8983")
	collection := env("SOLR_COLLECTION", "byz")
	kafkaEnabled := envBool("BYZ_KAFKA_ENABLED", true)
	kafkaBootstrap := env("KAFKA_BOOTSTRAP", "127.0.0.1:9092")
	searchTopic := env("KAFKA_TOPIC_SEARCH_QUERY", "byz.search.query")
	maxSize := envInt("SEARCH_MAX_SIZE", defaultMaxSize)
	if maxSize < 1 {
		maxSize = defaultMaxSize
	}
	docMaxChars := envInt("DOCUMENT_MAX_CHARS", defaultDocMaxChars)
	if docMaxChars < 10_000 {
		docMaxChars = 10_000
	}

	logs := NewLogBuffer()
	teeStdLog(logs, "byz-search")

	solr := NewSolrClient(solrURL, collection)
	publisher, err := NewKafkaPublisher(splitCSV(kafkaBootstrap), kafkaEnabled, searchTopic)
	if err != nil {
		log.Fatalf("kafka: %v", err)
	}
	defer publisher.Close()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	jwks, err := keyfunc.NewDefaultCtx(ctx, []string{jwksURL})
	if err != nil {
		log.Fatalf("jwks: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/actuator/health", withCORS(func(w http.ResponseWriter, r *http.Request) {
		status := http.StatusOK
		body := map[string]any{"status": "UP", "documentMaxChars": docMaxChars}
		pingCtx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := solr.Ping(pingCtx); err != nil {
			status = http.StatusServiceUnavailable
			body = map[string]any{"status": "DOWN", "solr": err.Error()}
		}
		writeJSON(w, status, body)
	}))
	mux.HandleFunc("/healthz", withCORS(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "UP"})
	}))

	mux.HandleFunc("/api/v1/search", withCORS(withJWT(jwks, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		handleSearch(w, r, solr, publisher, maxSize)
	})))

	// Full indexed body for one doc (agent get_document). List search stays snippet-only.
	mux.HandleFunc("GET /api/v1/documents/{id}", withCORS(withJWT(jwks, func(w http.ResponseWriter, r *http.Request) {
		handleGetDocument(w, r, solr, r.PathValue("id"), docMaxChars)
	})))

	mux.HandleFunc("/api/v1/admin/logs", withCORS(withJWT(jwks, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		lines := 200
		if v := r.URL.Query().Get("lines"); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				lines = n
			}
		}
		writeJSON(w, http.StatusOK, logs.Tail(lines, r.URL.Query().Get("level")))
	})))

	addr := bind + ":" + port
	srv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		log.Printf("byz-search listening on http://%s solr=%s/%s kafka=%v docMaxChars=%d",
			addr, solrURL, collection, kafkaEnabled, docMaxChars)
		logs.Add("INFO", "byz-search", "listening on "+addr, nil)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http: %v", err)
		}
	}()

	<-ctx.Done()
	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdown)
	log.Printf("shutdown")
}

func handleSearch(w http.ResponseWriter, r *http.Request, solr *SolrClient, pub *KafkaPublisher, maxSize int) {
	claims, ok := claimsFrom(r.Context())
	if !ok {
		writeProblem(w, http.StatusUnauthorized, "Unauthorized", "missing claims")
		return
	}

	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeErr(w, errBadRequest("q is required"))
		return
	}
	if len(q) > 1000 {
		writeErr(w, errBadRequest("q too long (max 1000)"))
		return
	}

	page := 0
	if v := r.URL.Query().Get("page"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			writeErr(w, errBadRequest("page must be >= 0"))
			return
		}
		page = n
	}
	size := defaultSize
	if v := r.URL.Query().Get("size"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			writeErr(w, errBadRequest("size must be >= 1"))
			return
		}
		size = n
	}
	if size > maxSize {
		size = maxSize
	}

	// Platform admin (BYZ_ADMIN_ORGANIZATION_ID): can search on behalf of another org
	// (used by byz-agent with client_credentials). Empty admin org = allow override (local/dev).
	adminOrg := strings.TrimSpace(env("BYZ_ADMIN_ORGANIZATION_ID", ""))
	isPlatformAdmin := adminOrg == "" || claims.OrganizationID == adminOrg

	orgID := claims.OrganizationID
	if v := strings.TrimSpace(r.URL.Query().Get("organizationId")); v != "" {
		if !isPlatformAdmin {
			writeErr(w, errForbidden("organizationId override requires platform admin org"))
			return
		}
		orgID = v
	}

	// Tenant filter is opt-in via query param only. Admin uploads often have
	// null tenant_id; auto-filtering by JWT tenant would hide those docs.
	// Platform admin may pass any tenant (agent scopes from the chat run).
	tenantFilter := ""
	if v := strings.TrimSpace(r.URL.Query().Get("tenantId")); v != "" {
		if !isPlatformAdmin && claims.TenantID != "" && v != claims.TenantID {
			writeErr(w, errForbidden("tenantId does not match token"))
			return
		}
		tenantFilter = v
	}

	// Optional user visibility: shared (no user_id) OR owned by this user.
	// Non-admin may only set userId to themselves (sub or user_id claim).
	userFilter := ""
	if v := strings.TrimSpace(r.URL.Query().Get("userId")); v != "" {
		self := claims.UserID
		if self == "" {
			self = claims.Subject
		}
		if !isPlatformAdmin && self != "" && v != self {
			writeErr(w, errForbidden("userId does not match token subject"))
			return
		}
		userFilter = v
	}

	requestID := strings.TrimSpace(r.Header.Get("X-Request-Id"))
	if requestID == "" {
		requestID = uuid.NewString()
	}
	w.Header().Set("X-Request-Id", requestID)

	started := time.Now()
	total, hits, err := solr.Search(r.Context(), q, orgID, tenantFilter, userFilter, page, size)
	durationMs := time.Since(started).Milliseconds()
	if err != nil {
		log.Printf("solr search: %v", err)
		pub.PublishQuery(SearchQueryEvent{
			EventID:        uuid.NewString(),
			Type:           "search.query",
			OccurredAt:     time.Now().UTC().Format(time.RFC3339Nano),
			RequestID:      requestID,
			OrganizationID: orgID,
			TenantID:       tenantFilter,
			UserID:         claims.UserID,
			ClientID:       claims.ClientID,
			Q:              q,
			Page:           page,
			Size:           size,
			Total:          0,
			HitCount:       0,
			DurationMs:     durationMs,
			Status:         http.StatusBadGateway,
		})
		detail := "solr search failed"
		if msg := err.Error(); msg != "" {
			detail = truncate(msg, 280)
		}
		writeErr(w, errBadGateway(detail))
		return
	}
	if hits == nil {
		hits = []SearchHit{}
	}

	resp := SearchResponse{
		Q:          q,
		Page:       page,
		Size:       size,
		Total:      total,
		DurationMs: durationMs,
		Hits:       hits,
	}
	writeJSON(w, http.StatusOK, resp)

	pub.PublishQuery(SearchQueryEvent{
		EventID:        uuid.NewString(),
		Type:           "search.query",
		OccurredAt:     time.Now().UTC().Format(time.RFC3339Nano),
		RequestID:      requestID,
		OrganizationID: orgID,
		TenantID:       tenantFilter,
		UserID:         claims.UserID,
		ClientID:       claims.ClientID,
		Q:              q,
		Page:           page,
		Size:           size,
		Total:          total,
		HitCount:       len(hits),
		DurationMs:     durationMs,
		Status:         http.StatusOK,
	})
}

func handleGetDocument(w http.ResponseWriter, r *http.Request, solr *SolrClient, id string, maxChars int) {
	claims, ok := claimsFrom(r.Context())
	if !ok {
		writeProblem(w, http.StatusUnauthorized, "Unauthorized", "missing claims")
		return
	}
	id = strings.TrimSpace(id)
	if id == "" {
		writeErr(w, errBadRequest("document id required"))
		return
	}

	adminOrg := strings.TrimSpace(env("BYZ_ADMIN_ORGANIZATION_ID", ""))
	isPlatformAdmin := adminOrg == "" || claims.OrganizationID == adminOrg

	orgID := claims.OrganizationID
	if v := strings.TrimSpace(r.URL.Query().Get("organizationId")); v != "" {
		if !isPlatformAdmin {
			writeErr(w, errForbidden("organizationId override requires platform admin org"))
			return
		}
		orgID = v
	}

	userFilter := ""
	if v := strings.TrimSpace(r.URL.Query().Get("userId")); v != "" {
		self := claims.UserID
		if self == "" {
			self = claims.Subject
		}
		if !isPlatformAdmin && self != "" && v != self {
			writeErr(w, errForbidden("userId does not match token subject"))
			return
		}
		userFilter = v
	}

	doc, err := solr.GetByID(r.Context(), id, orgID, userFilter)
	if err != nil {
		log.Printf("solr get document id=%s: %v", id, err)
		writeErr(w, errBadGateway("solr document fetch failed"))
		return
	}
	if doc == nil {
		writeErr(w, errNotFound("document not found in scope"))
		return
	}

	content := anyString(doc[fieldBody])
	contentChars := len([]rune(content))
	truncated := false
	if maxChars > 0 && contentChars > maxChars {
		runes := []rune(content)
		content = string(runes[:maxChars])
		truncated = true
		contentChars = maxChars
	}

	writeJSON(w, http.StatusOK, DocumentResponse{
		ID:             anyString(doc[fieldID]),
		Title:          anyString(doc[fieldTitle]),
		Source:         anyString(doc[fieldSource]),
		Path:           anyString(doc[fieldPath]),
		OrganizationID: anyString(doc[fieldOrg]),
		TenantID:       anyString(doc[fieldTenant]),
		UserID:         anyString(doc[fieldUser]),
		Tags:           anyStringSlice(doc[fieldTags]),
		Content:        content,
		ContentChars:   contentChars,
		Truncated:      truncated,
		MaxChars:       maxChars,
	})
}

package handlers

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"

	"github.com/sirupsen/logrus"
)

// CustomEthNodeKey identifies a user-defined node by network+instance.
type CustomEthNodeKey struct {
	Network  string
	Instance string
}

// CustomEthNodeNode holds the upstream URLs and basic-auth credentials for a
// single user-defined node. Username/Password apply to both BeaconURL and
// ExecutionURL.
type CustomEthNodeNode struct {
	BeaconURL    string
	ExecutionURL string
	Username     string
	Password     string
}

// CustomEthNodeConfig holds the lookup table for user-defined Ethereum nodes.
type CustomEthNodeConfig struct {
	Nodes map[CustomEthNodeKey]CustomEthNodeNode
}

// CustomEthNodeHandler proxies requests to user-configured beacon and execution
// node endpoints. Unlike EthNodeHandler, upstream URLs are looked up from
// configuration rather than derived from the ethpandaops.io DNS naming
// convention. Per-node basic-auth credentials are attached to outgoing requests.
type CustomEthNodeHandler struct {
	log    logrus.FieldLogger
	nodes  map[CustomEthNodeKey]CustomEthNodeNode
	mu     sync.RWMutex
	proxes map[string]*httputil.ReverseProxy
}

// NewCustomEthNodeHandler creates a new custom Ethereum node handler.
func NewCustomEthNodeHandler(log logrus.FieldLogger, cfg CustomEthNodeConfig) *CustomEthNodeHandler {
	nodes := make(map[CustomEthNodeKey]CustomEthNodeNode, len(cfg.Nodes))
	for k, v := range cfg.Nodes {
		nodes[k] = v
	}

	return &CustomEthNodeHandler{
		log:    log.WithField("handler", "custom_ethnode"),
		nodes:  nodes,
		proxes: make(map[string]*httputil.ReverseProxy, 16),
	}
}

// ServeHTTP handles user-defined beacon and execution node requests.
// Path format: /custom/{beacon|execution}/{network}/{instance}/...
func (h *CustomEthNodeHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	var mode string

	switch {
	case strings.HasPrefix(path, "/custom/beacon/"):
		mode = "beacon"
		path = strings.TrimPrefix(path, "/custom/beacon/")
	case strings.HasPrefix(path, "/custom/execution/"):
		mode = "execution"
		path = strings.TrimPrefix(path, "/custom/execution/")
	default:
		http.Error(w, "invalid path: must start with /custom/beacon/ or /custom/execution/", http.StatusBadRequest)

		return
	}

	// Split remaining path into network/instance/rest.
	parts := strings.SplitN(path, "/", 3)
	if len(parts) < 2 {
		http.Error(w, "invalid path: must include /{network}/{instance}/...", http.StatusBadRequest)

		return
	}

	network := parts[0]
	instance := parts[1]

	rest := "/"
	if len(parts) == 3 && parts[2] != "" {
		rest = "/" + parts[2]
	}

	// Validate network and instance segments.
	if !validSegment.MatchString(network) {
		http.Error(w, "invalid network name: must match [a-z0-9-]", http.StatusBadRequest)

		return
	}

	if !validSegment.MatchString(instance) {
		http.Error(w, "invalid instance name: must match [a-z0-9-]", http.StatusBadRequest)

		return
	}

	node, ok := h.nodes[CustomEthNodeKey{Network: network, Instance: instance}]
	if !ok {
		http.Error(w, fmt.Sprintf("no custom ethnode configured for network=%s instance=%s", network, instance), http.StatusNotFound)

		return
	}

	upstreamRaw := node.BeaconURL
	if mode == "execution" {
		upstreamRaw = node.ExecutionURL
	}

	if upstreamRaw == "" {
		http.Error(w, fmt.Sprintf("no %s_url configured for network=%s instance=%s", mode, network, instance), http.StatusNotFound)

		return
	}

	upstream, err := url.Parse(upstreamRaw)
	if err != nil {
		h.log.WithError(err).WithFields(logrus.Fields{
			"mode":     mode,
			"network":  network,
			"instance": instance,
			"upstream": upstreamRaw,
		}).Error("Invalid upstream URL in custom ethnode config")
		http.Error(w, "invalid upstream URL in proxy config", http.StatusInternalServerError)

		return
	}

	proxy := h.getOrCreateProxy(upstream, node.Username, node.Password)

	// Concatenate the upstream's configured path (if any) with the request path
	// so users can configure URLs like https://gw.example.com/cl-1.
	r.URL.Path = joinURLPath(upstream.Path, rest)

	h.log.WithFields(logrus.Fields{
		"mode":     mode,
		"network":  network,
		"instance": instance,
		"path":     r.URL.Path,
		"method":   r.Method,
		"upstream": upstream.Host,
	}).Debug("Proxying custom ethnode request")

	proxy.ServeHTTP(w, r)
}

// getOrCreateProxy returns a cached reverse proxy for the upstream, creating
// one if needed. Cache key is scheme+host so EL/CL on the same host share a
// transport while preserving correct routing.
func (h *CustomEthNodeHandler) getOrCreateProxy(upstream *url.URL, username, password string) *httputil.ReverseProxy {
	cacheKey := upstream.Scheme + "://" + upstream.Host

	h.mu.RLock()
	proxy, ok := h.proxes[cacheKey]
	h.mu.RUnlock()

	if ok {
		return proxy
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	if proxy, ok = h.proxes[cacheKey]; ok {
		return proxy
	}

	target := &url.URL{
		Scheme: upstream.Scheme,
		Host:   upstream.Host,
	}

	rp := httputil.NewSingleHostReverseProxy(target)
	rp.Transport = newProxyTransport(false)

	originalDirector := rp.Director
	rp.Director = func(req *http.Request) {
		originalDirector(req)

		// Remove the sandbox's Authorization header (Bearer token).
		req.Header.Del("Authorization")

		if username != "" {
			req.SetBasicAuth(username, password)
		}

		req.Host = req.URL.Host
		req.Header.Del("Host")
	}

	rp.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, err error) {
		h.log.WithError(err).WithField("upstream", target.Host).Error("Proxy error")
		http.Error(w, fmt.Sprintf("proxy error: %v", err), http.StatusBadGateway)
	}

	h.proxes[cacheKey] = rp

	return rp
}

// joinURLPath joins an upstream base path with the per-request path, ensuring
// exactly one slash between them.
func joinURLPath(base, rest string) string {
	switch {
	case base == "" || base == "/":
		return rest
	case rest == "" || rest == "/":
		return base
	}

	return strings.TrimRight(base, "/") + "/" + strings.TrimLeft(rest, "/")
}

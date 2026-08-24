package proxy

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/tavp-stack/tavpbox/internal/certs"
)

type Route struct {
	Domain string `json:"domain"`
	IP     string `json:"ip"`
	Port   int    `json:"port"`
}

type Proxy struct {
	mu     sync.RWMutex
	routes []Route
	port   int
}

func New(port int) *Proxy {
	return &Proxy{port: port}
}

// routesOverride lets tests redirect the routes file (empty = default).
var routesOverride string

func (p *Proxy) routesFile() string {
	if routesOverride != "" {
		return routesOverride
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".tavpbox", "proxy", "routes.json")
}

func (p *Proxy) loadRoutes() {
	data, err := os.ReadFile(p.routesFile())
	if err != nil {
		return
	}
	json.Unmarshal(data, &p.routes)
}

func (p *Proxy) saveRoutes() error {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".tavpbox", "proxy")
	if routesOverride != "" {
		dir = filepath.Dir(routesOverride)
	}
	os.MkdirAll(dir, 0755)
	data := []byte("[]\n")
	if p.routes == nil {
		p.routes = []Route{}
	} else {
		var err error
		data, err = json.MarshalIndent(p.routes, "", "  ")
		if err != nil {
			return err
		}
	}
	return os.WriteFile(p.routesFile(), data, 0644)
}

func (p *Proxy) AddRoute(domain, ip string, port int) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Load existing routes from disk first
	p.loadRoutesFromDisk()

	// Remove existing route for this domain
	var newRoutes []Route
	for _, r := range p.routes {
		if r.Domain != domain {
			newRoutes = append(newRoutes, r)
		}
	}
	newRoutes = append(newRoutes, Route{Domain: domain, IP: ip, Port: port})
	p.routes = newRoutes
	p.saveRoutes()
}

func (p *Proxy) loadRoutesFromDisk() {
	data, err := os.ReadFile(p.routesFile())
	if err != nil {
		return
	}
	if string(data) == "null" || len(strings.TrimSpace(string(data))) == 0 {
		return
	}
	json.Unmarshal(data, &p.routes)
}

func (p *Proxy) RemoveRoute(domain string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Load from disk first: callers typically use a fresh Proxy instance,
	// and saving an empty in-memory list would wipe ALL routes (#24).
	p.loadRoutesFromDisk()

	var newRoutes []Route
	for _, r := range p.routes {
		if r.Domain != domain {
			newRoutes = append(newRoutes, r)
		}
	}
	p.routes = newRoutes
	p.saveRoutes()
}

func (p *Proxy) Routes() []Route {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.routes
}

func (p *Proxy) Start(domainSuffix string) error {
	p.loadRoutes()

	// Watch routes.json for changes
	go p.watchRoutes()

	mux := http.NewServeMux()
	mux.HandleFunc("/", p.handler)

	fmt.Printf("TAVPBox proxy HTTP on :%d\n", p.port)

	// HTTPS on :443 using a self-managed local CA (auto-created and trusted)
	if domainSuffix != "" {
		go func() {
			// Establishes the CA and user trust (one-time); per-host certs
			// are then issued on demand via GetCertificate/SNI (#26).
			if _, err := certs.EnsureWildcard(domainSuffix); err != nil {
				fmt.Printf("TAVPBox proxy: HTTPS disabled (%v)\n", err)
				return
			}
			srv := &http.Server{
				Addr:    ":443",
				Handler: mux,
				TLSConfig: &tls.Config{
					MinVersion: tls.VersionTLS12,
					GetCertificate: func(hi *tls.ClientHelloInfo) (*tls.Certificate, error) {
						return certs.ForHost(hi.ServerName, domainSuffix)
					},
				},
			}
			fmt.Printf("TAVPBox proxy HTTPS on :443 (*.%s)\n", domainSuffix)
			if err := srv.ListenAndServeTLS("", ""); err != nil {
				fmt.Printf("TAVPBox proxy: HTTPS listener stopped (%v)\n", err)
			}
		}()
	}

	return http.ListenAndServe(fmt.Sprintf(":%d", p.port), mux)
}

// watchRoutes periodically checks for changes to routes.json
func (p *Proxy) watchRoutes() {
	var lastMod time.Time
	routesFile := p.routesFile()

	for {
		time.Sleep(2 * time.Second)
		info, err := os.Stat(routesFile)
		if err != nil {
			continue
		}
		if info.ModTime() != lastMod {
			lastMod = info.ModTime()
			p.loadRoutes()
		}
	}
}

// lookupRoute finds the route for a Host header. Exact match first; then
// falls back to stripping leading labels so multi-level subdomains
// (e.g. app.penbill.tavp.my.id) reach the project route (penbill.tavp.my.id)
// — see #26.
func (p *Proxy) lookupRoute(host string) *Route {
	host = strings.ToLower(host)
	if i := strings.LastIndex(host, ":"); i != -1 {
		host = host[:i]
	}
	for depth := 0; depth < 5 && host != ""; depth++ {
		for i := range p.routes {
			if p.routes[i].Domain == host {
				return &p.routes[i]
			}
		}
		dot := strings.Index(host, ".")
		if dot <= 0 {
			break
		}
		host = host[dot+1:]
	}
	return nil
}

func (p *Proxy) handler(w http.ResponseWriter, r *http.Request) {
	p.mu.RLock()
	route := p.lookupRoute(r.Host)
	p.mu.RUnlock()

	if route == nil {
		http.Error(w, "TAVPBox — No project configured for "+r.Host, http.StatusNotFound)
		return
	}

	target := fmt.Sprintf("http://%s:%d", route.IP, route.Port)
	proxyURL, err := url.Parse(target)
	if err != nil {
		http.Error(w, "Bad gateway", http.StatusBadGateway)
		return
	}

	proxy := httputil.NewSingleHostReverseProxy(proxyURL)
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		http.Error(w, "Service unavailable", http.StatusServiceUnavailable)
	}
	proxy.ServeHTTP(w, r)
}

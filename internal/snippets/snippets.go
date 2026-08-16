// Package snippets provides code snippet templates for Go development.
package snippets

import (
	"fmt"
	"strings"
)

// Snippet represents a code snippet template.
type Snippet struct {
	Name        string
	Description string
	Category    string
	Template    string
	Variables   []string
}

// Registry holds all available snippets.
type Registry struct {
	snippets map[string]Snippet
}

// NewRegistry creates a new snippet registry with built-in templates.
func NewRegistry() *Registry {
	r := &Registry{
		snippets: make(map[string]Snippet),
	}
	r.loadBuiltInSnippets()
	return r
}

// loadBuiltInSnippets loads the built-in Go snippet templates.
func (r *Registry) loadBuiltInSnippets() {
	builtIns := []Snippet{
		{
			Name:        "main",
			Description: "Standard main function with error handling",
			Category:    "boilerplate",
			Template: `package main

import (
	"fmt"
	"os"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// Your code here
	return nil
}`,
			Variables: []string{},
		},
		{
			Name:        "http_server",
			Description: "Basic HTTP server with graceful shutdown",
			Category:    "web",
			Template: `package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/", handleRoot)
	
	server := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}
	
	go func() {
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()
	
	fmt.Println("Server starting on :8080")
	
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("shutdown error: %v", err)
	}
	
	fmt.Println("Server stopped")
}

func handleRoot(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	fmt.Fprintln(w, "Hello, World!")
}`,
			Variables: []string{"port"},
		},
		{
			Name:        "cli_app",
			Description: "Basic CLI application structure",
			Category:    "cli",
			Template: `package main

import (
	"flag"
	"fmt"
	"os"
)

type Config struct {
	Verbose bool
	Output  string
}

func main() {
	cfg := parseFlags()
	
	if err := run(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func parseFlags() *Config {
	cfg := &Config{}
	flag.BoolVar(&cfg.Verbose, "v", false, "verbose output")
	flag.StringVar(&cfg.Output, "o", "", "output file")
	flag.Parse()
	return cfg
}

func run(cfg *Config) error {
	if cfg.Verbose {
		fmt.Println("Running in verbose mode")
	}
	
	// Your code here
	return nil
}`,
			Variables: []string{},
		},
		{
			Name:        "struct_json",
			Description: "Struct with JSON tags and methods",
			Category:    "types",
			Template: `type ${NAME} struct {
	ID   int    ` + "`" + `json:"id"` + "`" + `
	Name string ` + "`" + `json:"name"` + "`" + `
}

func New${NAME}(id int, name string) *${NAME} {
	return &${NAME}{
		ID:   id,
		Name: name,
	}
}

func (n *${NAME}) String() string {
	return fmt.Sprintf("${NAME}{ID: %d, Name: %s}", n.ID, n.Name)
}`,
			Variables: []string{"NAME"},
		},
		{
			Name:        "interface_repo",
			Description: "Repository interface pattern",
			Category:    "patterns",
			Template: `// ${ENTITY}Repository defines the interface for ${ENTITY} data access.
type ${ENTITY}Repository interface {
	GetByID(ctx context.Context, id int) (*${ENTITY}, error)
	List(ctx context.Context, opts ListOptions) ([]*${ENTITY}, error)
	Create(ctx context.Context, e *${ENTITY}) error
	Update(ctx context.Context, e *${ENTITY}) error
	Delete(ctx context.Context, id int) error
}

// ${ENTITY}Service provides business logic for ${ENTITY}.
type ${ENTITY}Service struct {
	repo ${ENTITY}Repository
}

func New${ENTITY}Service(repo ${ENTITY}Repository) *${ENTITY}Service {
	return &${ENTITY}Service{repo: repo}
}`,
			Variables: []string{"ENTITY"},
		},
		{
			Name:        "test_function",
			Description: "Standard test function template",
			Category:    "testing",
			Template: `func Test${NAME}(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:    "success case",
			input:   "valid input",
			want:    "expected output",
			wantErr: false,
		},
		{
			name:    "error case",
			input:   "invalid input",
			want:    "",
			wantErr: true,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := FunctionUnderTest(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("FunctionUnderTest() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("FunctionUnderTest() = %v, want %v", got, tt.want)
			}
		})
	}
}`,
			Variables: []string{"NAME"},
		},
		{
			Name:        "goroutine_worker",
			Description: "Worker pool pattern with goroutines",
			Category:    "concurrency",
			Template: `type Job struct {
	ID   int
	Data interface{}
}

type WorkerPool struct {
	jobs    chan Job
	results chan Result
	numWorkers int
}

type Result struct {
	JobID int
	Output interface{}
	Error  error
}

func NewWorkerPool(numWorkers int) *WorkerPool {
	return &WorkerPool{
		jobs:       make(chan Job, 100),
		results:    make(chan Result, 100),
		numWorkers: numWorkers,
	}
}

func (wp *WorkerPool) Start(ctx context.Context) {
	for i := 0; i < wp.numWorkers; i++ {
		go wp.worker(ctx, i)
	}
}

func (wp *WorkerPool) worker(ctx context.Context, id int) {
	for job := range wp.jobs {
		result := processJob(ctx, job)
		wp.results <- result
	}
}

func processJob(ctx context.Context, job Job) Result {
	// Process the job
	return Result{JobID: job.ID, Output: nil, Error: nil}
}

func (wp *WorkerPool) Submit(job Job) {
	wp.jobs <- job
}

func (wp *WorkerPool) Results() <-chan Result {
	return wp.results
}`,
			Variables: []string{},
		},
		{
			Name:        "middleware_chain",
			Description: "HTTP middleware chain",
			Category:    "web",
			Template: `// Middleware is a function that wraps an http.Handler.
type Middleware func(http.Handler) http.Handler

// Chain applies middlewares to a handler in order.
func Chain(h http.Handler, middlewares ...Middleware) http.Handler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		h = middlewares[i](h)
	}
	return h
}

// LoggingMiddleware logs request information.
func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %v", r.Method, r.URL.Path, time.Since(start))
	})
}

// RecoveryMiddleware recovers from panics.
func RecoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}`,
			Variables: []string{},
		},
		{
			Name:        "error_handling",
			Description: "Custom error types and wrapping",
			Category:    "errors",
			Template: `// Error types
var (
	ErrNotFound = errors.New("not found")
	ErrInvalid  = errors.New("invalid input")
)

// AppError represents an application error.
type AppError struct {
	Code    string
	Message string
	Err     error
}

func (e *AppError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *AppError) Unwrap() error {
	return e.Err
}

// NewAppError creates a new application error.
func NewAppError(code, message string, err error) *AppError {
	return &AppError{
		Code:    code,
		Message: message,
		Err:     err,
	}
}`,
			Variables: []string{},
		},
		{
			Name:        "config_loader",
			Description: "Configuration loading from env and file",
			Category:    "utility",
			Template: `type Config struct {
	ServerPort int    ` + "`" + `env:"SERVER_PORT" envDefault:"8080"` + "`" + `
	DatabaseURL string ` + "`" + `env:"DATABASE_URL" required:"true"` + "`" + `
	LogLevel   string ` + "`" + `env:"LOG_LEVEL" envDefault:"info"` + "`" + `
}

func LoadConfig() (*Config, error) {
	cfg := &Config{}
	
	// Load from environment
	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("failed to parse env: %w", err)
	}
	
	// Optionally load from file
	if configFile := os.Getenv("CONFIG_FILE"); configFile != "" {
		if err := loadFromFile(configFile, cfg); err != nil {
			return nil, fmt.Errorf("failed to load config file: %w", err)
		}
	}
	
	return cfg, nil
}

func loadFromFile(path string, cfg *Config) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, cfg)
}`,
			Variables: []string{},
		},
	}

	for _, s := range builtIns {
		r.snippets[s.Name] = s
	}
}

// Get retrieves a snippet by name.
func (r *Registry) Get(name string) (Snippet, bool) {
	snippet, ok := r.snippets[name]
	return snippet, ok
}

// List returns all snippet names, optionally filtered by category.
func (r *Registry) List(category string) []Snippet {
	var result []Snippet
	for _, s := range r.snippets {
		if category == "" || s.Category == category {
			result = append(result, s)
		}
	}
	return result
}

// Categories returns all available categories.
func (r *Registry) Categories() []string {
	cats := make(map[string]bool)
	for _, s := range r.snippets {
		cats[s.Category] = true
	}
	result := make([]string, 0, len(cats))
	for c := range cats {
		result = append(result, c)
	}
	return result
}

// Render renders a snippet template with variable substitutions.
func (r *Registry) Render(name string, vars map[string]string) (string, error) {
	snippet, ok := r.Get(name)
	if !ok {
		return "", fmt.Errorf("snippet not found: %s", name)
	}

	template := snippet.Template
	for key, value := range vars {
		template = strings.ReplaceAll(template, "${"+key+"}", value)
	}

	return template, nil
}

// Add adds a custom snippet to the registry.
func (r *Registry) Add(snippet Snippet) {
	r.snippets[snippet.Name] = snippet
}

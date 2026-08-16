package snippets

import (
	"fmt"
	"strings"
)

// Snippet represents a code snippet template.
type Snippet struct {
	Name        string
	Category    string
	Description string
	Template    string
	Variables   []string
}

// Registry manages code snippets.
type Registry struct {
	snippets map[string]*Snippet
}

// NewRegistry creates a new snippet registry with built-in snippets.
func NewRegistry() *Registry {
	r := &Registry{
		snippets: make(map[string]*Snippet),
	}
	r.registerBuiltInSnippets()
	return r
}

// registerBuiltInSnippets registers all built-in Go snippets.
func (r *Registry) registerBuiltInSnippets() {
	// Main function template
	r.Register(&Snippet{
		Name:        "main",
		Category:    "basic",
		Description: "Basic main function with error handling",
		Template: `package main

import (
	"fmt"
	"os"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// Your code here
	return nil
}`,
	})

	// HTTP Server template
	r.Register(&Snippet{
		Name:        "http_server",
		Category:    "web",
		Description: "Basic HTTP server with routing",
		Template: `package main

import (
	"encoding/json"
	"log"
	"net/http"
)

func main() {
	http.HandleFunc("/", handleHome)
	http.HandleFunc("/api/", handleAPI)
	
	log.Println("Server starting on :8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatal(err)
	}
}

func handleHome(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Welcome!"))
}

func handleAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "ok",
	})
}`,
	})

	// CLI App template
	r.Register(&Snippet{
		Name:        "cli_app",
		Category:    "cli",
		Description: "Basic CLI application structure",
		Template: `package main

import (
	"flag"
	"fmt"
	"os"
)

var (
	version = "1.0.0"
	verbose bool
	output  string
)

func init() {
	flag.BoolVar(&verbose, "v", false, "verbose output")
	flag.StringVar(&output, "o", "", "output file")
	flag.BoolVar(&verbose, "verbose", false, "verbose output")
	flag.StringVar(&output, "output", "", "output file")
}

func main() {
	flag.Parse()
	
	if verbose {
		fmt.Printf("Starting with verbose mode, output: %s\n", output)
	}
	
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	args := flag.Args()
	if len(args) == 0 {
		return fmt.Errorf("no input provided")
	}
	
	// Process args
	for _, arg := range args {
		if verbose {
			fmt.Printf("Processing: %s\n", arg)
		}
	}
	
	return nil
}`,
	})

	// Struct with JSON tags
	r.Register(&Snippet{
		Name:        "struct_json",
		Category:    "types",
		Description: "Struct with JSON marshaling",
		Template: `type {{.Name}} struct {
	ID        int64  ` + "`json:\"id\"`" + `
	Name      string ` + "`json:\"name\"`" + `
	Email     string ` + "`json:\"email,omitempty\"`" + `
	CreatedAt int64  ` + "`json:\"created_at\"`" + `
	UpdatedAt int64  ` + "`json:\"updated_at\"`" + `
}`,
		Variables: []string{"Name"},
	})

	// Interface with repository pattern
	r.Register(&Snippet{
		Name:        "interface_repo",
		Category:    "patterns",
		Description: "Repository interface pattern",
		Template: `// {{.Entity}}Repository defines the interface for {{.Entity}} data access.
type {{.Entity}}Repository interface {
	GetByID(ctx context.Context, id int64) (*{{.Entity}}, error)
	GetAll(ctx context.Context) ([]*{{.Entity}}, error)
	Create(ctx context.Context, e *{{.Entity}}) error
	Update(ctx context.Context, e *{{.Entity}}) error
	Delete(ctx context.Context, id int64) error
}

// {{.Entity}}Service provides business logic for {{.Entity}}.
type {{.Entity}}Service struct {
	repo {{.Entity}}Repository
}

func New{{.Entity}}Service(repo {{.Entity}}Repository) *{{.Entity}}Service {
	return &{{.Entity}}Service{repo: repo}
}`,
		Variables: []string{"Entity"},
	})

	// Test function template
	r.Register(&Snippet{
		Name:        "test_function",
		Category:    "testing",
		Description: "Table-driven test template",
		Template: `func Test{{.FunctionName}}(t *testing.T) {
	tests := []struct {
		name    string
		input   {{.InputType}}
		want    {{.OutputType}}
		wantErr bool
	}{
		{
			name:    "success case",
			input:   /* setup */,
			want:    /* expected */,
			wantErr: false,
		},
		{
			name:    "error case",
			input:   /* setup */,
			want:    /* expected */,
			wantErr: true,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := {{.FunctionName}}(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("{{.FunctionName}}() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("{{.FunctionName}}() = %v, want %v", got, tt.want)
			}
		})
	}
}`,
		Variables: []string{"FunctionName", "InputType", "OutputType"},
	})

	// Goroutine worker pool
	r.Register(&Snippet{
		Name:        "goroutine_worker",
		Category:    "concurrency",
		Description: "Worker pool with goroutines",
		Template: `type Job struct {
	ID   int
	Data interface{}
}

type WorkerPool struct {
	jobs    chan Job
	results chan error
	wg      sync.WaitGroup
}

func NewWorkerPool(numWorkers int) *WorkerPool {
	wp := &WorkerPool{
		jobs:    make(chan Job, 100),
		results: make(chan error, 100),
	}
	
	for i := 0; i < numWorkers; i++ {
		wp.wg.Add(1)
		go wp.worker(i)
	}
	
	return wp
}

func (wp *WorkerPool) worker(id int) {
	defer wp.wg.Done()
	
	for job := range wp.jobs {
		if err := wp.process(job); err != nil {
			wp.results <- fmt.Errorf("worker %d: job %d failed: %w", id, job.ID, err)
		}
	}
}

func (wp *WorkerPool) process(job Job) error {
	// Process the job
	return nil
}

func (wp *WorkerPool) Submit(job Job) {
	wp.jobs <- job
}

func (wp *WorkerPool) Close() {
	close(wp.jobs)
	wp.wg.Wait()
	close(wp.results)
}`,
	})

	// Middleware chain
	r.Register(&Snippet{
		Name:        "middleware_chain",
		Category:    "web",
		Description: "HTTP middleware chain",
		Template: `type Middleware func(http.Handler) http.Handler

func Chain(h http.Handler, middlewares ...Middleware) http.Handler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		h = middlewares[i](h)
	}
	return h
}

func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %v", r.Method, r.URL.Path, time.Since(start))
	})
}

func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("Authorization")
		if token == "" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}`,
	})

	// Error handling wrapper
	r.Register(&Snippet{
		Name:        "error_handling",
		Category:    "patterns",
		Description: "Custom error types with wrapping",
		Template: `type Error struct {
	Code    string
	Message string
	Err     error
}

func (e *Error) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *Error) Unwrap() error {
	return e.Err
}

func NewError(code, message string) *Error {
	return &Error{Code: code, Message: message}
}

func WrapError(err error, code, message string) *Error {
	return &Error{Code: code, Message: message, Err: err}
}

func IsNotFound(err error) bool {
	var e *Error
	if errors.As(err, &e) {
		return e.Code == "NOT_FOUND"
	}
	return false
}`,
	})

	// Config loader
	r.Register(&Snippet{
		Name:        "config_loader",
		Category:    "utility",
		Description: "Configuration loading from env and file",
		Template: `type Config struct {
	ServerAddr   string ` + "`env:\"SERVER_ADDR\" envDefault:\":8080\"`" + `
	DatabaseURL  string ` + "`env:\"DATABASE_URL,required\"`" + `
	LogLevel     string ` + "`env:\"LOG_LEVEL\" envDefault:\"info\"`" + `
	Environment  string ` + "`env:\"ENV\" envDefault:\"development\"`" + `
}

func LoadConfig() (*Config, error) {
	cfg := &Config{}
	
	// Load from environment
	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("failed to parse env: %w", err)
	}
	
	// Override from file if exists
	if cfgPath := os.Getenv("CONFIG_FILE"); cfgPath != "" {
		if err := loadFromFile(cfg, cfgPath); err != nil {
			return nil, fmt.Errorf("failed to load config file: %w", err)
		}
	}
	
	return cfg, nil
}

func loadFromFile(cfg *Config, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, cfg)
}`,
	})

	// SQL database connection
	r.Register(&Snippet{
		Name:        "sql_database",
		Category:    "database",
		Description: "SQL database connection with migration",
		Template: `package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"
	
	_ "github.com/lib/pq"
)

type DB struct {
	*sql.DB
}

func NewDB(dsn string) (*DB, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)
	
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}
	
	return &DB{db}, nil
}

func (db *DB) Migrate(ctx context.Context) error {
	query := ` + "`" + `
	CREATE TABLE IF NOT EXISTS users (
		id SERIAL PRIMARY KEY,
		email VARCHAR(255) UNIQUE NOT NULL,
		created_at TIMESTAMP DEFAULT NOW()
	);
	` + "`" + `
	
	_, err := db.ExecContext(ctx, query)
	return err
}`,
	})
}

// Register adds a snippet to the registry.
func (r *Registry) Register(snippet *Snippet) {
	r.snippets[snippet.Name] = snippet
}

// Get retrieves a snippet by name.
func (r *Registry) Get(name string) (*Snippet, error) {
	snippet, ok := r.snippets[name]
	if !ok {
		return nil, fmt.Errorf("snippet not found: %s", name)
	}
	return snippet, nil
}

// List returns all snippets, optionally filtered by category.
func (r *Registry) List(category string) []*Snippet {
	var result []*Snippet
	
	for _, s := range r.snippets {
		if category == "" || s.Category == category {
			result = append(result, s)
		}
	}
	
	// Sort by name
	for i := 0; i < len(result)-1; i++ {
		for j := i + 1; j < len(result); j++ {
			if result[i].Name > result[j].Name {
				result[i], result[j] = result[j], result[i]
			}
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

// Render renders a snippet with variable substitutions.
func (r *Registry) Render(name string, vars map[string]string) (string, error) {
	snippet, err := r.Get(name)
	if err != nil {
		return "", err
	}
	
	result := snippet.Template
	
	// Simple variable substitution {{.VarName}}
	for key, value := range vars {
		placeholder := "{{." + key + "}}"
		result = strings.ReplaceAll(result, placeholder, value)
	}
	
	return result, nil
}

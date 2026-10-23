package main

import (
	"errors"
	"io"
	"net/http"
	"sync"
	"time"
	"strings"
	"strconv"
	"sort"
	"encoding/json"
)

type Task struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Done      bool      `json:"done"`
	UpdatedAt time.Time `json:"updatedAt"`
}
type TaskRepo interface {
	Create(title string) (Task, error)
	Get(id string) (Task, bool)
	List(done bool) []Task
	SetDone(id string, done bool) (Task, error)
}
type Clock interface{ Now() time.Time }

var (
	ErrNotFound     = errors.New("task not found")
	ErrInvalidTitle = errors.New("invalid title")
)

type inMemoryTaskRepo struct {
	mu    sync.RWMutex
	clock Clock
	seq   uint64
	tasks map[string]Task
}

func NewInMemoryTaskRepo(clock Clock) TaskRepo {
	return &inMemoryTaskRepo{
		clock: clock,
		tasks: make(map[string]Task),
	}
}
func (r *inMemoryTaskRepo) Create(title string) (Task, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return Task{}, ErrInvalidTitle
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	
	r.seq++
	id := strconv.FormatUint(r.seq, 10)
	task := Task{
		ID: id,
		Title: title,
		Done: false,
		UpdatedAt: r.clock.Now(),
	}
	r.tasks[id] = task
	return task, nil
}
func (r *inMemoryTaskRepo) Get(id string) (Task, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	task, ok := r.tasks[id]
	if !ok {
		return Task{}, false
	}
	return task, true
}
func (r *inMemoryTaskRepo) List(done bool) []Task {
	results := []Task{}
	r.mu.RLock()
	for _, task := range r.tasks {
		if task.Done == done {
			results = append(results, task)
		}
	}
	r.mu.RUnlock()
	sort.Slice(results, func(i, j int) bool {
		if results[i].UpdatedAt.Equal(results[j].UpdatedAt) {
			return results[i].ID < results[j].ID
		}
		return results[i].UpdatedAt.After(results[j].UpdatedAt)
	})
	return results
}
func (r *inMemoryTaskRepo) SetDone(id string, done bool) (Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	task, ok := r.tasks[id]
	if !ok {
		return Task{}, ErrNotFound
	}

	if task.Done == done {
		return task, nil
	}

	task.Done = done
	task.UpdatedAt = r.clock.Now()
	r.tasks[id] = task
	return task, nil
}

type httpHandler struct{ repo TaskRepo }

func NewHTTPHandler(repo TaskRepo) http.Handler {
	return &httpHandler{repo: repo}
}
func (h *httpHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if path == "/tasks" {
		switch r.Method {
			case http.MethodPost:
				h.handleCreate(w, r)
			case http.MethodGet:
				h.handleList(w, r)
			default:
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
		return
	}

	if strings.HasPrefix(path, "/tasks/") {
		id := strings.TrimPrefix(path, "/tasks/")
		if id == "" || strings.Contains(id, "/") {
			http.NotFound(w, r)
			return
		}
		switch r.Method {
			case http.MethodGet:
				h.handleGet(w, r, id)
			case http.MethodPatch:
				h.handlePatch(w, r, id)
			default:
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
		return
	}

	http.NotFound(w, r)
}
func (h *httpHandler) handleCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Title *string `json:"title"`
	}
	if err := decodeStrictJSON(r.Body, &req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Title == nil {
		http.Error(w, "Title is required", http.StatusBadRequest)
		return
	}

	task, err := h.repo.Create(*req.Title)
	if err != nil {
		if errors.Is(err, ErrInvalidTitle) {
			http.Error(w, "Invalid title", http.StatusBadRequest)
			return
		}
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, task)
}
func (h *httpHandler) handleGet(w http.ResponseWriter, r *http.Request, id string) {
	task, ok := h.repo.Get(id)
	if !ok {
		http.Error(w, "Task not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, task)
}
func (h *httpHandler) handleList(w http.ResponseWriter, r *http.Request) {
	doneFlag := false
	query := r.URL.Query()
	values, ok := query["done"]
	if ok {
		if len(values) != 1 {
			http.Error(w, "Invalid done parameter", http.StatusBadRequest)
			return
		}
		switch values[0] {
			case "true":
				doneFlag = true
			case "false":
			default:
				http.Error(w, "Invalid done parameter", http.StatusBadRequest)
			return
		}
	}
	tasks := h.repo.List(doneFlag)
	writeJSON(w, http.StatusOK, tasks)
}
func (h *httpHandler) handlePatch(w http.ResponseWriter, r *http.Request, id string) {
	var req struct {
		Done *bool `json:"done"`
	}
	if err := decodeStrictJSON(r.Body, &req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Done == nil {
		http.Error(w, "Done is required", http.StatusBadRequest)
		return
	}

	task, err := h.repo.SetDone(id, *req.Done)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			http.Error(w, "Task not found", http.StatusNotFound)
			return
		}

        http.Error(w, "Internal error", http.StatusInternalServerError)
        return
	}
	writeJSON(w, http.StatusOK, task)
}
func decodeStrictJSON(r io.Reader, v any) error {
	body, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	text := strings.TrimSpace(string(body))
	if !strings.HasPrefix(text, "{") {
		return errors.New("invalid JSON object")
	}
	decoder := json.NewDecoder(strings.NewReader(text))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(v); err != nil {
		return err
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return errors.New("Extra data in JSON")
	}
	return nil
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	data, err := json.Marshal(v)
	if err != nil {
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(data)
}

package storage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type RunRecord struct {
	ID           string     `json:"id"`
	ProjectName  string     `json:"project_name"`
	WorkflowName string     `json:"workflow_name"`
	Status       string     `json:"status"`
	CreatedAt    time.Time  `json:"created_at"`
	FinishedAt   *time.Time `json:"finished_at,omitempty"`
}

type StepRecord struct {
	RunID    string `json:"run_id"`
	StepName string `json:"step_name"`
	State    string `json:"state"`
	Data     string `json:"data"`
	Error    string `json:"error"`
	Duration float64 `json:"duration"`
	Target   string `json:"target"`
}

type LogEntry struct {
	RunID     string `json:"run_id"`
	StepName  string `json:"step_name"`
	Timestamp string `json:"timestamp"`
	Level     string `json:"level"`
	Message   string `json:"message"`
}

// RunDoc 一次执行的完整文档
type RunDoc struct {
	Run   RunRecord   `json:"run"`
	Steps []StepRecord `json:"steps"`
	Logs  []LogEntry   `json:"logs"`
}

type Store struct {
	mu     sync.Mutex
	dir    string
	runs   map[string]*RunDoc
}

var globalStore *Store

func GetStore() *Store { return globalStore }

// Init 初始化 JSON 文件存储
func Init(baseDir string) error {
	dir := filepath.Join(baseDir, "data", "runs")
	os.MkdirAll(dir, 0755)

	store := &Store{
		dir:  dir,
		runs: make(map[string]*RunDoc),
	}

	// 加载已有记录
	files, _ := filepath.Glob(filepath.Join(dir, "*.json"))
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var doc RunDoc
		if json.Unmarshal(data, &doc) == nil {
			store.runs[doc.Run.ID] = &doc
		}
	}

	globalStore = store
	return nil
}

func (s *Store) saveDoc(id string) {
	doc, ok := s.runs[id]
	if !ok {
		return
	}
	data, _ := json.MarshalIndent(doc, "", "  ")
	os.WriteFile(filepath.Join(s.dir, id+".json"), data, 0644)
}

func (s *Store) SaveRun(run RunRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.runs[run.ID] = &RunDoc{Run: run, Steps: []StepRecord{}, Logs: []LogEntry{}}
	s.saveDoc(run.ID)
	return nil
}

func (s *Store) UpdateRunStatus(id, status string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, ok := s.runs[id]
	if !ok {
		return nil
	}
	doc.Run.Status = status
	if status == "success" || status == "failed" {
		now := time.Now()
		doc.Run.FinishedAt = &now
	}
	s.saveDoc(id)
	return nil
}

func (s *Store) SaveStep(rec StepRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, ok := s.runs[rec.RunID]
	if !ok {
		return nil
	}
	// 如果已存在该步骤，更新
	for i, step := range doc.Steps {
		if step.StepName == rec.StepName {
			doc.Steps[i] = rec
			s.saveDoc(rec.RunID)
			return nil
		}
	}
	doc.Steps = append(doc.Steps, rec)
	s.saveDoc(rec.RunID)
	return nil
}

func (s *Store) AppendLog(entry LogEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, ok := s.runs[entry.RunID]
	if !ok {
		return nil
	}
	doc.Logs = append(doc.Logs, entry)
	// 每10条日志存一次盘，减少 IO
	if len(doc.Logs)%10 == 0 {
		s.saveDoc(entry.RunID)
	}
	return nil
}

func (s *Store) GetHistory() ([]RunRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	runs := make([]RunRecord, 0, len(s.runs))
	for _, doc := range s.runs {
		runs = append(runs, doc.Run)
	}
	// 按时间降序
	for i := 0; i < len(runs); i++ {
		for j := i + 1; j < len(runs); j++ {
			if runs[j].CreatedAt.After(runs[i].CreatedAt) {
				runs[i], runs[j] = runs[j], runs[i]
			}
		}
	}
	if len(runs) > 100 {
		runs = runs[:100]
	}
	return runs, nil
}

func (s *Store) GetRunDetail(id string) (*RunDoc, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, ok := s.runs[id]
	if !ok {
		return nil, os.ErrNotExist
	}
	return doc, nil
}

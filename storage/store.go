package storage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
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

func (s *Store) Flush(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.saveDoc(id)
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

func (s *Store) UpdateStepDuration(runID, stepName string, duration float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, ok := s.runs[runID]
	if !ok {
		return
	}
	for i, step := range doc.Steps {
		if step.StepName == stepName {
			doc.Steps[i].Duration = duration
			s.saveDoc(runID)
			return
		}
	}
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

// HistoryFilter 历史查询过滤条件
type HistoryFilter struct {
	Status  string // 空=全部
	Project string // 空=全部
	Search  string // 搜索 RunID 或项目名
	Page    int    // 页码，从1开始
	Size    int    // 每页条数
}

type HistoryResult struct {
	Runs  []RunRecord `json:"runs"`
	Total int         `json:"total"`
	Page  int         `json:"page"`
	Size  int         `json:"size"`
}

func (s *Store) GetHistory(filter *HistoryFilter) (*HistoryResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	runs := make([]RunRecord, 0, len(s.runs))
	for _, doc := range s.runs {
		runs = append(runs, doc.Run)
	}

	// 排序：按时间降序
	sort.Slice(runs, func(i, j int) bool {
		return runs[j].CreatedAt.After(runs[i].CreatedAt)
	})

	// 过滤
	filtered := make([]RunRecord, 0)
	for _, r := range runs {
		if filter.Status != "" && r.Status != filter.Status {
			continue
		}
		if filter.Project != "" && r.ProjectName != filter.Project {
			continue
		}
		if filter.Search != "" {
			search := strings.ToLower(filter.Search)
			if !strings.Contains(strings.ToLower(r.ID), search) &&
				!strings.Contains(strings.ToLower(r.ProjectName), search) {
				continue
			}
		}
		filtered = append(filtered, r)
	}

	total := len(filtered)

	// 分页
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.Size < 1 {
		filter.Size = 20
	}
	start := (filter.Page - 1) * filter.Size
	if start > len(filtered) {
		start = len(filtered)
	}
	end := start + filter.Size
	if end > len(filtered) {
		end = len(filtered)
	}

	return &HistoryResult{
		Runs:  filtered[start:end],
		Total: total,
		Page:  filter.Page,
		Size:  filter.Size,
	}, nil
}

func (s *Store) GetHistorySimple() ([]RunRecord, error) {
	result, _ := s.GetHistory(&HistoryFilter{Page: 1, Size: 100})
	if result == nil {
		return []RunRecord{}, nil
	}
	return result.Runs, nil
}

func (s *Store) DeleteRun(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.runs, id)
	os.Remove(filepath.Join(s.dir, id+".json"))
	return nil
}

func (s *Store) DeleteRuns(ids []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, id := range ids {
		delete(s.runs, id)
		os.Remove(filepath.Join(s.dir, id+".json"))
	}
	return nil
}

func (s *Store) GetLatestRun(projectName string) *RunRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	var latest *RunRecord
	for _, doc := range s.runs {
		if doc.Run.ProjectName == projectName {
			if latest == nil || doc.Run.CreatedAt.After(latest.CreatedAt) {
				latest = &doc.Run
			}
		}
	}
	return latest
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

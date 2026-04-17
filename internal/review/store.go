package review

import (
	"sync"
	"time"

	"github.com/jqtmviyu/iinaServer/internal/model"
)

type Store struct {
	mu       sync.RWMutex
	byID     map[string]*model.SessionReview
	order    []string
	maxItems int
}

func NewStore(maxItems int) *Store {
	if maxItems <= 0 {
		maxItems = 20
	}
	return &Store{
		byID:     make(map[string]*model.SessionReview),
		maxItems: maxItems,
	}
}

func (s *Store) Create(sessionID string, source string) *model.SessionReview {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	review := &model.SessionReview{
		SessionID: sessionID,
		Source:    source,
		Status:    "created",
		CreatedAt: now,
		UpdatedAt: now,
	}
	s.byID[sessionID] = review
	s.order = append([]string{sessionID}, s.order...)
	s.compactLocked()
	clone := cloneReview(review)
	return &clone
}

func (s *Store) Get(sessionID string) (*model.SessionReview, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	review, ok := s.byID[sessionID]
	if !ok {
		return nil, false
	}
	clone := cloneReview(review)
	return &clone, true
}

func (s *Store) SetPlan(sessionID string, plan model.PlayPlan) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if review, ok := s.byID[sessionID]; ok {
		review.Plan = &plan
		review.UpdatedAt = time.Now()
	}
}

func (s *Store) SetStatus(sessionID string, status string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if review, ok := s.byID[sessionID]; ok {
		review.Status = status
		review.UpdatedAt = time.Now()
	}
}

func (s *Store) SetLaunch(sessionID string, args []string, socket string, pid int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if review, ok := s.byID[sessionID]; ok {
		review.IINAArgs = append([]string(nil), args...)
		review.IPCSocket = socket
		review.PID = pid
		review.UpdatedAt = time.Now()
	}
}

func (s *Store) AddSample(sessionID string, sample model.ProgressSample) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if review, ok := s.byID[sessionID]; ok {
		review.Samples = append(review.Samples, sample)
		if len(review.Samples) > 30 {
			review.Samples = review.Samples[len(review.Samples)-30:]
		}
		review.UpdatedAt = time.Now()
	}
}

func (s *Store) AddUpload(sessionID string, upload model.UploadRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if review, ok := s.byID[sessionID]; ok {
		review.Uploads = append(review.Uploads, upload)
		if len(review.Uploads) > 50 {
			review.Uploads = review.Uploads[len(review.Uploads)-50:]
		}
		review.UpdatedAt = time.Now()
	}
}

func (s *Store) AddError(sessionID string, err string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if review, ok := s.byID[sessionID]; ok {
		review.Errors = append(review.Errors, err)
		if len(review.Errors) > 20 {
			review.Errors = review.Errors[len(review.Errors)-20:]
		}
		review.UpdatedAt = time.Now()
	}
}

func (s *Store) SetMetadata(sessionID string, metadata map[string]any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if review, ok := s.byID[sessionID]; ok {
		review.Metadata = cloneAnyMap(metadata)
		review.UpdatedAt = time.Now()
	}
}

func (s *Store) Latest() (*model.SessionReview, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.order) == 0 {
		return nil, false
	}
	review, ok := s.byID[s.order[0]]
	if !ok {
		return nil, false
	}
	clone := cloneReview(review)
	return &clone, true
}

func (s *Store) compactLocked() {
	seen := make(map[string]struct{}, len(s.order))
	filtered := s.order[:0]
	for _, id := range s.order {
		if _, ok := seen[id]; ok {
			continue
		}
		if _, ok := s.byID[id]; !ok {
			continue
		}
		seen[id] = struct{}{}
		filtered = append(filtered, id)
	}
	s.order = filtered
	for len(s.order) > s.maxItems {
		last := s.order[len(s.order)-1]
		delete(s.byID, last)
		s.order = s.order[:len(s.order)-1]
	}
}

func cloneReview(in *model.SessionReview) model.SessionReview {
	out := *in
	if in.Plan != nil {
		plan := *in.Plan
		if in.Plan.RequestHeaders != nil {
			plan.RequestHeaders = cloneStringMap(in.Plan.RequestHeaders)
		}
		out.Plan = &plan
	}
	out.IINAArgs = append([]string(nil), in.IINAArgs...)
	out.Samples = append([]model.ProgressSample(nil), in.Samples...)
	out.Errors = append([]string(nil), in.Errors...)
	out.Metadata = cloneAnyMap(in.Metadata)
	if len(in.Uploads) > 0 {
		out.Uploads = make([]model.UploadRecord, 0, len(in.Uploads))
		for _, upload := range in.Uploads {
			clone := upload
			clone.Query = cloneStringMap(upload.Query)
			clone.Headers = cloneStringMap(upload.Headers)
			clone.Body = cloneAnyMap(upload.Body)
			out.Uploads = append(out.Uploads, clone)
		}
	}
	return out
}

func cloneStringMap(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneAnyMap(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

package server

import (
	"crypto/rand"
	"encoding/hex"
	"io"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/sneiko/goleo/media"
)

type assetRecord struct {
	ID          string
	Name        string
	Size        int64
	ContentType string
	Path        string
	LastAccess  time.Time
}

func (record assetRecord) browserValue() media.AudioAsset {
	return media.AudioAsset{
		ID:          record.ID,
		Name:        record.Name,
		Size:        record.Size,
		ContentType: record.ContentType,
		URL:         "/api/assets/" + record.ID,
	}
}

func (record assetRecord) handlerValue() media.AudioInput {
	return media.AudioInput{
		ID:          record.ID,
		Name:        record.Name,
		Size:        record.Size,
		ContentType: record.ContentType,
		Path:        record.Path,
		URL:         "/api/assets/" + record.ID,
	}
}

type assetStore struct {
	mu        sync.RWMutex
	dir       string
	records   map[string]assetRecord
	ttl       time.Duration
	closeOnce sync.Once
}

func newAssetStore() (*assetStore, error) {
	dir, err := os.MkdirTemp("", "goleo-assets-*")
	if err != nil {
		return nil, err
	}

	store := &assetStore{
		dir:     dir,
		records: map[string]assetRecord{},
		ttl:     30 * time.Minute,
	}

	return store, nil
}

func (store *assetStore) create(name string, contentType string, reader io.Reader) (assetRecord, error) {
	fileName := sanitizeFileName(name)
	if fileName == "" {
		fileName = "asset"
	}

	id := "asset-" + randomToken(12)
	destinationPath := filepath.Join(store.dir, id+"-"+fileName)
	destination, err := os.OpenFile(destinationPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return assetRecord{}, err
	}

	size, copyErr := io.Copy(destination, reader)
	closeErr := destination.Close()
	if copyErr != nil {
		return assetRecord{}, copyErr
	}
	if closeErr != nil {
		return assetRecord{}, closeErr
	}

	record := assetRecord{
		ID:          id,
		Name:        name,
		Size:        size,
		ContentType: normalizeContentType(contentType, name),
		Path:        destinationPath,
		LastAccess:  time.Now(),
	}

	store.mu.Lock()
	expiredPaths := store.cleanupExpiredLocked(time.Now())
	store.records[id] = record
	store.mu.Unlock()

	for _, path := range expiredPaths {
		_ = os.Remove(path)
	}

	return record, nil
}

func (store *assetStore) createFromPath(output media.AudioOutput) (assetRecord, error) {
	source, err := os.Open(output.Path)
	if err != nil {
		return assetRecord{}, err
	}
	defer source.Close()

	name := output.Name
	if name == "" {
		name = filepath.Base(output.Path)
	}

	return store.create(name, output.ContentType, source)
}

func (store *assetStore) get(id string) (assetRecord, bool) {
	store.mu.Lock()
	defer store.mu.Unlock()

	record, ok := store.records[id]
	if !ok {
		return assetRecord{}, false
	}

	if store.ttl > 0 && time.Since(record.LastAccess) > store.ttl {
		delete(store.records, id)
		_ = os.Remove(record.Path)
		return assetRecord{}, false
	}

	record.LastAccess = time.Now()
	store.records[id] = record
	return record, true
}

func (store *assetStore) close() error {
	var closeErr error
	store.closeOnce.Do(func() {
		closeErr = os.RemoveAll(store.dir)
	})

	return closeErr
}

func (store *assetStore) cleanupExpired(now time.Time) {
	if store == nil || store.ttl <= 0 {
		return
	}

	expiredPaths := make([]string, 0)

	store.mu.Lock()
	expiredPaths = store.cleanupExpiredLocked(now)
	store.mu.Unlock()

	for _, path := range expiredPaths {
		_ = os.Remove(path)
	}
}

func (store *assetStore) cleanupExpiredLocked(now time.Time) []string {
	if store == nil || store.ttl <= 0 {
		return nil
	}

	expiredPaths := make([]string, 0)
	for id, record := range store.records {
		if now.Sub(record.LastAccess) <= store.ttl {
			continue
		}

		delete(store.records, id)
		expiredPaths = append(expiredPaths, record.Path)
	}

	return expiredPaths
}

func normalizeContentType(contentType string, name string) string {
	contentType = strings.TrimSpace(contentType)
	if contentType != "" {
		return contentType
	}

	if byExt := mime.TypeByExtension(filepath.Ext(name)); byExt != "" {
		return byExt
	}

	return "application/octet-stream"
}

func randomToken(length int) string {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "fallback"
	}

	token := hex.EncodeToString(bytes)
	if len(token) > length {
		return token[:length]
	}
	return token
}

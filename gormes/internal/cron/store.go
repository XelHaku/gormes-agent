package cron

import (
	"encoding/json"
	"errors"
	"fmt"

	"go.etcd.io/bbolt"
)

// ErrJobNotFound is returned by Get / Delete / Update when the target
// job ID isn't in the cron_jobs bucket.
var ErrJobNotFound = errors.New("cron: job not found")

// ErrJobNameTaken is returned by Create when another job already uses
// the requested Name. Names are unique; IDs are unique too but
// separately (IDs are random, names are operator-assigned).
var ErrJobNameTaken = errors.New("cron: job name already taken")

const cronJobsBucket = "cron_jobs"

// Store is the bbolt-backed Job persistence layer. The underlying
// *bbolt.DB is owned by the caller (typically the same *bbolt.DB the
// Phase 2.C session map uses, so a single file on disk).
type Store struct {
	db *bbolt.DB
}

// NewStore opens/creates the cron_jobs bucket and returns a ready-to-use
// Store. Safe to call multiple times.
func NewStore(db *bbolt.DB) (*Store, error) {
	err := db.Update(func(tx *bbolt.Tx) error {
		_, e := tx.CreateBucketIfNotExists([]byte(cronJobsBucket))
		return e
	})
	if err != nil {
		return nil, fmt.Errorf("cron: init bucket: %w", err)
	}
	return &Store{db: db}, nil
}

// Create persists a new job. Fails with ErrJobNameTaken if Name is
// already used.
func (s *Store) Create(j Job) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(cronJobsBucket))
		var dup bool
		_ = b.ForEach(func(k, v []byte) error {
			var other Job
			if err := json.Unmarshal(v, &other); err != nil {
				return nil
			}
			if other.Name == j.Name {
				dup = true
			}
			return nil
		})
		if dup {
			return ErrJobNameTaken
		}
		blob, err := json.Marshal(j)
		if err != nil {
			return err
		}
		return b.Put([]byte(j.ID), blob)
	})
}

// Get loads one job by ID.
func (s *Store) Get(id string) (Job, error) {
	var j Job
	err := s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(cronJobsBucket))
		blob := b.Get([]byte(id))
		if blob == nil {
			return ErrJobNotFound
		}
		return json.Unmarshal(blob, &j)
	})
	return j, err
}

// List returns every job in the bucket. Corrupt rows are silently
// skipped so one bad blob doesn't block operation.
func (s *Store) List() ([]Job, error) {
	var out []Job
	err := s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(cronJobsBucket))
		return b.ForEach(func(k, v []byte) error {
			var j Job
			if err := json.Unmarshal(v, &j); err != nil {
				return nil // skip corrupt
			}
			out = append(out, j)
			return nil
		})
	})
	return out, err
}

// Update overwrites an existing job by ID. Errors with ErrJobNotFound
// if the ID isn't present.
func (s *Store) Update(j Job) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(cronJobsBucket))
		if b.Get([]byte(j.ID)) == nil {
			return ErrJobNotFound
		}
		blob, err := json.Marshal(j)
		if err != nil {
			return err
		}
		return b.Put([]byte(j.ID), blob)
	})
}

// Delete removes a job by ID. No-op on missing keys (bbolt convention).
func (s *Store) Delete(id string) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(cronJobsBucket))
		return b.Delete([]byte(id))
	})
}

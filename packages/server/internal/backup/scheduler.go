package backup

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
)

const autoNamePrefix = "auto-"

// StartScheduler creates a backup on the cron spec (5 fields, standard
// syntax) and prunes the oldest auto- backups down to keep, mirroring
// Manual backups are never pruned.
func (s *Service) StartScheduler(spec string, keep int) error {
	c := cron.New()
	_, err := c.AddFunc(spec, func() {
		ctx := context.Background()
		name := autoNamePrefix + time.Now().UTC().Format("2006-01-02T15-04-05Z")
		if err := s.Create(ctx, name); err != nil {
			s.log.Error("scheduled backup failed", "err", err)
			return
		}
		s.log.Info("scheduled backup created", "name", name)
		if keep > 0 {
			if err := s.pruneAuto(ctx, keep); err != nil {
				s.log.Error("backup retention failed", "err", err)
			}
		}
	})
	if err != nil {
		return fmt.Errorf("backup: invalid schedule %q: %w", spec, err)
	}
	c.Start()
	s.cronStoper = func() { c.Stop() }
	return nil
}

func (s *Service) StopScheduler() {
	if s.cronStoper != nil {
		s.cronStoper()
	}
}

func (s *Service) pruneAuto(ctx context.Context, keep int) error {
	backups, err := s.List(ctx)
	if err != nil {
		return err
	}
	var autos []Backup
	for _, b := range backups {
		if strings.HasPrefix(b.Name, autoNamePrefix) {
			autos = append(autos, b)
		}
	}
	if len(autos) <= keep {
		return nil
	}
	sort.Slice(autos, func(i, j int) bool { return autos[i].Modified.After(autos[j].Modified) })
	for _, b := range autos[keep:] {
		if err := s.Delete(ctx, b.Name); err != nil {
			return err
		}
		s.log.Info("backup pruned by retention", "name", b.Name)
	}
	return nil
}

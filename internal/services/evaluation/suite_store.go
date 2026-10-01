// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

// suitesRootDir is where custom suites persist. Suites are authored content,
// not campaign evidence: they are neither campaign-scoped nor archived.
func suitesRootDir() string {
	return filepath.Join(evalRoot(), constants.EvaluationSuitesDirname)
}

func suitePath(id string) string {
	return filepath.Join(suitesRootDir(), id+constants.FileExtJSON)
}

// ResolveSuite returns the suite with the given ID: a built-in suite, or a
// custom one from the suite store. The second result reports a built-in suite.
func (s *Store) ResolveSuite(ctx context.Context, id string) (ScenarioSuite, bool, error) {
	if def, ok := BuiltinScenarioSuite(id); ok {
		return def, true, nil
	}
	def, err := s.loadCustomSuite(ctx, id)
	return def, false, err
}

// LoadSuiteCatalog materializes the suite with the given ID into the catalog
// and fixture artifacts a campaign freezes. Built-in suites go through their
// own strict gates; a custom suite is validated against the full scenario
// contract as it materializes.
func (s *Store) LoadSuiteCatalog(ctx context.Context, id string) (*evalv1.EvaluationScenarioCatalog, map[string]ScenarioArtifacts, error) {
	switch id {
	case DefaultSuiteID:
		return LoadScenarioCatalog()
	case SmokeSuiteID:
		return LoadSmokeGateScenarioCatalog()
	}
	def, err := s.loadCustomSuite(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	return MaterializeSuite(def)
}

// ListSuites returns every built-in suite followed by every custom suite in ID
// order. A stored file that no longer decodes is reported, never skipped, so a
// corrupt suite cannot silently disappear from the listing.
func (s *Store) ListSuites(ctx context.Context) ([]SuiteSummary, error) {
	if s == nil || s.files == nil {
		return nil, fmt.Errorf("%w: file service is required", constants.ErrEvidenceArtifactMalformed)
	}
	summaries := make([]SuiteSummary, 0, len(BuiltinSuiteIDs()))
	for _, id := range BuiltinSuiteIDs() {
		def, _ := BuiltinScenarioSuite(id)
		summaries = append(summaries, def.Summary(true))
	}
	entries, err := s.files.ReadDir(ctx, suitesRootDir())
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) || errors.Is(err, constants.ErrNotFound) {
			return summaries, nil
		}
		return nil, fmt.Errorf("evaluation: list suites: %w", err)
	}
	custom := make([]SuiteSummary, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, constants.FileExtJSON) {
			continue
		}
		def, err := s.loadCustomSuite(ctx, strings.TrimSuffix(name, constants.FileExtJSON))
		if err != nil {
			return nil, fmt.Errorf("evaluation: list suites: %w", err)
		}
		custom = append(custom, def.Summary(false))
	}
	sort.Slice(custom, func(i, j int) bool { return custom[i].ID < custom[j].ID })
	return append(summaries, custom...), nil
}

// CreateSuite validates and stores a new custom suite. It fails when a suite
// with that ID already exists, built-in or custom.
func (s *Store) CreateSuite(ctx context.Context, def ScenarioSuite) error {
	if err := s.requireSuiteStore(); err != nil {
		return err
	}
	if _, _, err := MaterializeSuite(def); err != nil {
		return err
	}
	if _, builtin := BuiltinScenarioSuite(def.ID); builtin {
		return fmt.Errorf("evaluation: create suite %q: %w", def.ID, constants.ErrEvaluationSuiteBuiltin)
	}
	exists, err := s.files.FileExists(ctx, suitePath(def.ID))
	if err != nil {
		return fmt.Errorf("evaluation: create suite %q: %w", def.ID, err)
	}
	if exists {
		return fmt.Errorf("evaluation: create suite %q: %w", def.ID, constants.ErrEvaluationSuiteExists)
	}
	return s.writeSuite(ctx, def)
}

// UpdateSuite replaces an existing custom suite. Changing a suite's content
// requires a new version, so one id@version never names two different
// catalogs; resubmitting identical content is a no-op.
func (s *Store) UpdateSuite(ctx context.Context, def ScenarioSuite) error {
	if err := s.requireSuiteStore(); err != nil {
		return err
	}
	if _, builtin := BuiltinScenarioSuite(def.ID); builtin {
		return fmt.Errorf("evaluation: update suite %q: %w", def.ID, constants.ErrEvaluationSuiteBuiltin)
	}
	next, _, err := MaterializeSuite(def)
	if err != nil {
		return err
	}
	current, err := s.loadCustomSuite(ctx, def.ID)
	if err != nil {
		return err
	}
	previous, _, err := MaterializeSuite(current)
	if err != nil {
		return fmt.Errorf("evaluation: update suite %q: stored suite is invalid: %w", def.ID, err)
	}
	if previous.GetCatalogDigest() == next.GetCatalogDigest() {
		return nil
	}
	if current.Version == def.Version {
		return suiteInvalid("suite %s@%s already exists with different content; change version to publish the edit", def.ID, def.Version)
	}
	return s.writeSuite(ctx, def)
}

// DeleteSuite removes a custom suite. Campaigns freeze their own copy of the
// suite they used, so deleting it never affects an existing campaign.
func (s *Store) DeleteSuite(ctx context.Context, id string) error {
	if err := s.requireSuiteStore(); err != nil {
		return err
	}
	if _, builtin := BuiltinScenarioSuite(id); builtin {
		return fmt.Errorf("evaluation: delete suite %q: %w", id, constants.ErrEvaluationSuiteBuiltin)
	}
	if _, err := s.loadCustomSuite(ctx, id); err != nil {
		return err
	}
	if err := s.files.Remove(ctx, suitePath(id)); err != nil {
		return fmt.Errorf("evaluation: delete suite %q: %w", id, err)
	}
	return nil
}

func (s *Store) requireSuiteStore() error {
	if s == nil || s.files == nil {
		return fmt.Errorf("%w: file service is required", constants.ErrEvaluationReportPersistFailed)
	}
	return nil
}

func (s *Store) loadCustomSuite(ctx context.Context, id string) (ScenarioSuite, error) {
	if s == nil || s.files == nil || !suiteIDPattern.MatchString(id) {
		return ScenarioSuite{}, fmt.Errorf("evaluation: load suite %q: %w", id, constants.ErrNotFound)
	}
	body, err := s.files.ReadFile(ctx, suitePath(id))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) || errors.Is(err, constants.ErrNotFound) {
			return ScenarioSuite{}, fmt.Errorf("evaluation: load suite %q: %w", id, constants.ErrNotFound)
		}
		return ScenarioSuite{}, fmt.Errorf("evaluation: load suite %q: %w", id, err)
	}
	def, err := DecodeScenarioSuite(body)
	if err != nil {
		return ScenarioSuite{}, fmt.Errorf("evaluation: load suite %q: %w", id, err)
	}
	if def.ID != id {
		return ScenarioSuite{}, fmt.Errorf("evaluation: load suite %q: %w: stored suite names id %q", id, constants.ErrEvaluationSuiteInvalid, def.ID)
	}
	return def, nil
}

func (s *Store) writeSuite(ctx context.Context, def ScenarioSuite) error {
	body, err := EncodeScenarioSuite(def)
	if err != nil {
		return err
	}
	path := suitePath(def.ID)
	if err := s.files.MkdirAll(ctx, filepath.Dir(path), constants.PermDirStandard); err != nil {
		return fmt.Errorf("%w: create suites directory: %w", constants.ErrEvaluationReportPersistFailed, err)
	}
	if err := s.files.WriteFile(ctx, path, body, constants.PermFileReadOnly); err != nil {
		return fmt.Errorf("%w: write suite %q: %w", constants.ErrEvaluationReportPersistFailed, def.ID, err)
	}
	return nil
}

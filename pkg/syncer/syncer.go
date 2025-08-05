// Package syncer implements types to sync charts between repositories
package syncer

import (
	goerrors "errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/bitnami/charts-syncer/api"
	"github.com/bitnami/charts-syncer/pkg/client"
	cs "github.com/bitnami/charts-syncer/pkg/client/source"
	ct "github.com/bitnami/charts-syncer/pkg/client/target"
	"github.com/bitnami/charts-syncer/pkg/client/types"
	"github.com/juju/errors"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/log"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/log/silent"
	"k8s.io/klog"
)

// Clients holds the source and target chart repo clients
type Clients struct {
	src client.ChartsWrapper
	dst client.ChartsUnwrapper
}

// A Syncer can be used to sync a source and target chart repos.
type Syncer struct {
	source *api.Source
	target *api.Target
	cli    *Clients

	dryRun            bool
	autoDiscovery     bool
	fromDate          string
	insecure          bool
	usePlainHTTP      bool
	latestVersionOnly bool

	// list of charts to skip
	skipCharts []string

	// list of container platforms to sync
	containerPlatforms []string

	// cherry picked charts with specific versions
	cherryPickedCharts []*api.CherryPickedChart

	// TODO(jdrios): Cache index in local filesystem to speed
	// up re-runs
	index ChartIndex

	// skip syncing artifacts
	skipArtifacts bool

	// skip syncing images
	skipImages bool

	// Storage directory for required artifacts
	workdir string

	logger log.SectionLogger
}

// Option is an option value used to create a new syncer instance.
type Option func(*Syncer)

// WithDryRun configures the syncer to run in dry-run mode.
func WithDryRun(enable bool) Option {
	return func(s *Syncer) {
		s.dryRun = enable
	}
}

// WithLogger configures the syncer to use a specific logger.
func WithLogger(l log.SectionLogger) Option {
	return func(s *Syncer) {
		s.logger = l
	}
}

// WithAutoDiscovery configures the syncer to discover all the charts to sync
// from the source chart repos.
func WithAutoDiscovery(enable bool) Option {
	return func(s *Syncer) {
		s.autoDiscovery = enable
	}
}

// WithFromDate configures the syncer to synchronize the charts from a specific
// time using YYYY-MM-DD format.
func WithFromDate(date string) Option {
	return func(s *Syncer) {
		s.fromDate = date
	}
}

// WithUsePlainHTTP configures the syncer to use plain HTTP
func WithUsePlainHTTP(enable bool) Option {
	return func(s *Syncer) {
		s.usePlainHTTP = enable
	}
}

// WithSkipArtifacts configures the syncer to skip syncing artifacts
func WithSkipArtifacts(skip bool) Option {
	return func(s *Syncer) {
		s.skipArtifacts = skip
	}
}

// WithSkipImages configures the syncer to skip syncing images
func WithSkipImages(skip bool) Option {
	return func(s *Syncer) {
		s.skipImages = skip
	}
}

// WithWorkdir configures the syncer to store artifacts in a specific directory.
func WithWorkdir(dir string) Option {
	return func(s *Syncer) {
		s.workdir = dir
	}
}

// WithInsecure configures the syncer to allow insecure SSL connections
func WithInsecure(enable bool) Option {
	return func(s *Syncer) {
		s.insecure = enable
	}
}

// WithLatestVersionOnly configures the syncer to sync only the latest version
func WithLatestVersionOnly(latestVersionOnly bool) Option {
	return func(s *Syncer) {
		s.latestVersionOnly = latestVersionOnly
	}
}

// WithSkipCharts configures the syncer to skip an explicit list of chart names
// from the source chart repos.
func WithSkipCharts(charts []string) Option {
	return func(s *Syncer) {
		s.skipCharts = charts
	}
}

// WithContainerPlatforms configures the syncer to sync chart containers for only
// the specified list of platforms. Leaving a blank list syncs all.
func WithContainerPlatforms(platforms []string) Option {
	return func(s *Syncer) {
		s.containerPlatforms = platforms
	}
}

// WithCherryPickedCharts configures the syncer to sync only specific versions
// of the specified charts.
func WithCherryPickedCharts(charts []*api.CherryPickedChart) Option {
	return func(s *Syncer) {
		s.cherryPickedCharts = charts
	}
}

// New creates a new syncer using Client
func New(source *api.Source, target *api.Target, opts ...Option) (*Syncer, error) {
	s := &Syncer{
		source: source,
		target: target,
		logger: silent.NewSectionLogger(),
	}

	for _, o := range opts {
		o(s)
	}

	// If a workdir wasn't specified, let's use a directory relative to the
	// current directory
	if s.workdir == "" {
		s.workdir = "./workdir"
	}

	klog.V(3).Infof("Using workdir: %q", s.workdir)

	if err := os.MkdirAll(s.workdir, 0755); err != nil {
		return nil, errors.Trace(err)
	}

	s.cli = &Clients{}

	if source.GetRepo() != nil {
		srcCli, err := cs.NewClient(source, types.WithCache(s.workdir), types.WithInsecure(s.insecure), types.WithUsePlainHTTP(s.usePlainHTTP))
		if err != nil {
			return nil, errors.Trace(err)
		}
		s.cli.src = srcCli
	} else {
		return nil, errors.New("no source info defined in config file")
	}

	if target.GetRepo() != nil {
		dstCli, err := ct.NewClient(target, types.WithCache(s.workdir), types.WithInsecure(s.insecure), types.WithUsePlainHTTP(s.usePlainHTTP))
		if err != nil {
			return nil, errors.Trace(err)
		}
		s.cli.dst = dstCli
	} else {
		return nil, errors.New("no target info defined in config file")
	}

	return s, nil
}

// SyncCherryPickedCharts syncs only the cherry picked charts with their specific versions
func (s *Syncer) SyncCherryPickedCharts() error {
	if len(s.cherryPickedCharts) == 0 {
		s.logger.Infof("No cherry picked charts defined")
		return ErrNoChartsToSync
	}

	s.logger.Infof("Found %d cherry picked chart definitions", len(s.cherryPickedCharts))

	// Build list of specific chart:version pairs that we want
	var specificCharts []*Chart

	// First, load the index to get available charts
	chartNames := make(map[string]bool)
	for _, cherryChart := range s.cherryPickedCharts {
		chartNames[cherryChart.GetName()] = true
	}

	var uniqueCharts []string
	for chartName := range chartNames {
		uniqueCharts = append(uniqueCharts, chartName)
	}

	s.logger.Infof("Loading charts for: %v", uniqueCharts)

	// Load charts from source
	if err := s.loadCharts(uniqueCharts...); err != nil {
		s.logger.Warnf("Problems loading charts: %v", err)
	}

	s.logger.Infof("Available charts in index: %d", len(s.getIndex()))

	// Now filter to get only the specific versions we want
	var notFoundVersions []string
	for _, cherryChart := range s.cherryPickedCharts {
		chartName := cherryChart.GetName()
		requestedVersions := cherryChart.GetVersions()

		s.logger.Infof("Processing cherry picked chart '%s' with %d requested versions", chartName, len(requestedVersions))

		for _, requestedVersion := range requestedVersions {
			chartKey := fmt.Sprintf("%s-%s", chartName, requestedVersion)

			if chart, exists := s.getIndex()[chartKey]; exists {
				s.logger.Infof("✓ Found matching chart: %s-%s", chart.Name, chart.Version)
				specificCharts = append(specificCharts, chart)
			} else {
				notFoundVersions = append(notFoundVersions, fmt.Sprintf("%s-%s", chartName, requestedVersion))
			}
		}
	}

	// Show summary of not found versions
	if len(notFoundVersions) > 0 {
		s.logger.Warnf("%d cherry picked chart versions not found in source or already synced:", len(notFoundVersions))
		for _, version := range notFoundVersions {
			s.logger.Warnf("  ✗ %s", version)
		}
	}

	// Display available chart versions in source for each requested chart
	s.displayAvailableVersions(uniqueCharts)

	if len(specificCharts) == 0 {
		s.logger.Warnf("No cherry picked charts found in source repository or all are synced")
		return ErrNoChartsToSync
	}

	s.logger.Infof("=== SYNCING %d CHERRY PICKED CHARTS ===", len(specificCharts))
	for i, chart := range specificCharts {
		s.logger.Infof("Will sync: %s-%s (%d/%d)", chart.Name, chart.Version, i+1, len(specificCharts))
	}

	// Now sync these specific charts
	return s.syncSpecificCharts(specificCharts)
}

// displayAvailableVersions shows all available versions for the requested charts in source
func (s *Syncer) displayAvailableVersions(chartNames []string) {
	s.logger.Infof("=== AVAILABLE VERSIONS IN SOURCE ===")

	for _, chartName := range chartNames {
		var versions []string

		// Collect all versions for this chart from the index
		for key, chart := range s.getIndex() {
			if strings.HasPrefix(key, chartName+"-") && chart.Name == chartName {
				versions = append(versions, chart.Version)
			}
		}

		// Sort versions
		sort.Strings(versions)

		if len(versions) > 0 {
			s.logger.Infof("Chart '%s' has %d available versions:", chartName, len(versions))
			for i, version := range versions {
				s.logger.Infof("  %d. %s", i+1, version)
			}
		} else {
			s.logger.Infof("Chart '%s': No versions available in source (may be all synced)", chartName)
		}
	}
	s.logger.Infof("=== END AVAILABLE VERSIONS ===")
}

// syncSpecificCharts syncs a specific list of Chart objects
func (s *Syncer) syncSpecificCharts(charts []*Chart) error {
	if len(charts) == 0 {
		return ErrNoChartsToSync
	}

	s.logger.Infof("Syncing %d specific chart versions", len(charts))

	var errs error
	for i, ch := range charts {
		id := fmt.Sprintf("%s-%s", ch.Name, ch.Version)
		s.logger.Infof("==> Syncing cherry picked chart %q (%d/%d)", id, i+1, len(charts))

		if err := s.logger.Section(fmt.Sprintf("==> Syncing %q chart (%d/%d)", id, i+1, len(charts)), func(l log.SectionLogger) error {
			return s.syncChart(ch, l)
		}); err != nil {
			s.logger.Warnf("Failed syncing %q chart: %v", id, err)
			errs = goerrors.Join(errs, errors.Trace(err))
		}
	}

	return errors.Trace(errs)
}

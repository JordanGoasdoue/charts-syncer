package main

import (
	"github.com/bitnami/charts-syncer/api"
	"github.com/bitnami/charts-syncer/internal/config"
	klogLogger "github.com/bitnami/charts-syncer/internal/log"
	"github.com/bitnami/charts-syncer/pkg/syncer"
	"github.com/juju/errors"
	"github.com/mitchellh/go-homedir"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/log"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/log/pterm"
	"k8s.io/klog"
)

var (
	syncFromDate          string
	syncWorkdir           string
	syncLatestVersionOnly bool
	usePlainHTTP          bool
)

var (
	syncExample = `
  # Synchronizes all charts defined in the configuration file
  charts-syncer sync

  # Synchronizes all charts defined in the configuration file from May 1st, 2020
  charts-syncer sync --from-date 2020-05-01`
)

func initConfigFile() error {
	// Use config file from the flag.
	if rootConfig != "" {
		viper.SetConfigFile(rootConfig)
		klog.Infof("Using config file: %q", rootConfig)
		return errors.Trace(viper.ReadInConfig())
	}

	// Find home directory.
	home, err := homedir.Dir()
	if err != nil {
		return errors.Trace(err)
	}

	// Search config in home directory with name ".charts-syncer" (without extension).
	viper.AddConfigPath(home)
	viper.AddConfigPath(".")
	viper.SetConfigName(defaultCfgFile)
	viper.SetConfigType("yaml")

	klog.Infof("Looking for the default config %s", defaultCfgFile)
	return errors.Trace(viper.ReadInConfig())
}

func newSyncCmd() *cobra.Command {
	var c api.Config
	usePlainLog := false

	cmd := &cobra.Command{
		Use:           "sync",
		Short:         "Synchronizes two chart repositories",
		Example:       syncExample,
		SilenceErrors: false,
		PreRunE: func(cmd *cobra.Command, _ []string) error {
			// Disable klog if we are using the pretty cui
			if !usePlainLog {
				_ = cmd.Flags().Lookup("alsologtostderr").Value.Set("false")
				_ = cmd.Flags().Lookup("logtostderr").Value.Set("false")
			}

			if err := initConfigFile(); err != nil {
				return errors.Trace(err)
			}

			// Env variables bindings for viper
			if err := config.InitEnvBindings(); err != nil {
				return errors.Trace(err)
			}

			// Load config file relying on viper to find it
			if err := config.Load(&c); err != nil {
				return errors.Trace(err)
			}

			if err := c.Validate(); err != nil {
				return errors.Trace(err)
			}

			return nil
		},
		RunE: func(_ *cobra.Command, _ []string) error {
			var parentLog log.SectionLogger

			if usePlainLog {
				parentLog = klogLogger.NewKlogSectionLogger()
			} else {
				parentLog = pterm.NewSectionLogger()
			}

			l := parentLog.StartSection("Syncing charts")

			syncerOptions := []syncer.Option{
				// TODO(jdrios): Some backends may not support discovery
				syncer.WithAutoDiscovery(true),
				syncer.WithDryRun(rootDryRun),
				syncer.WithFromDate(syncFromDate),
				syncer.WithWorkdir(syncWorkdir),
				syncer.WithContainerPlatforms(c.GetContainerPlatforms()),
				syncer.WithInsecure(rootInsecure),
				syncer.WithLatestVersionOnly(syncLatestVersionOnly),
				syncer.WithSkipArtifacts(c.GetSkipArtifacts()),
				syncer.WithSkipImages(c.GetSkipImages()),
				syncer.WithSkipCharts(c.SkipCharts),
				syncer.WithUsePlainHTTP(usePlainHTTP),
				syncer.WithLogger(l),
			}

			// Add cherry picked charts option if available
			if len(c.GetCherryPickedCharts()) > 0 {
				syncerOptions = append(syncerOptions, syncer.WithCherryPickedCharts(c.GetCherryPickedCharts()))
			}

			s, err := syncer.New(c.GetSource(), c.GetTarget(), syncerOptions...)
			if err != nil {
				return errors.Trace(err)
			}

			// Handle both regular charts and cherry picked charts
			hasRegularCharts := len(c.GetCharts()) > 0
			hasCherryPickedCharts := len(c.GetCherryPickedCharts()) > 0

			var syncError error

			// Create a map of cherry picked chart names for conflict detection
			cherryPickedNames := make(map[string]bool)
			if hasCherryPickedCharts {
				for _, chart := range c.GetCherryPickedCharts() {
					cherryPickedNames[chart.Name] = true
				}
			}

			// Sync regular charts if any, but exclude those that are in cherry picked
			if hasRegularCharts {
				var filteredCharts []string
				for _, chart := range c.GetCharts() {
					// Only include regular charts that are NOT in cherry picked list
					if !cherryPickedNames[chart] {
						filteredCharts = append(filteredCharts, chart)
					} else {
						l.Infof("Skipping regular chart '%s' because it's defined in cherry picked charts", chart)
					}
				}

				if len(filteredCharts) > 0 {
					l.Infof("Syncing %d regular charts (excluding cherry picked conflicts)", len(filteredCharts))
					if err := s.SyncPendingCharts(filteredCharts...); err != nil {
						if err != syncer.ErrNoChartsToSync {
							syncError = err
						}
					}
				}
			}

			// Sync cherry picked charts if any (these always take priority)
			if hasCherryPickedCharts && syncError == nil {
				l.Infof("Syncing %d cherry picked charts", len(c.GetCherryPickedCharts()))
				if err := s.SyncCherryPickedCharts(); err != nil {
					if err != syncer.ErrNoChartsToSync {
						syncError = err
					}
				}
			}

			// Handle the final result
			if syncError != nil {
				return l.Failf("Error syncing charts: %v", syncError)
			}

			// Check if anything was actually synced
			if !hasRegularCharts && !hasCherryPickedCharts {
				parentLog.Successf("There are no charts out of sync!")
				return nil
			}

			parentLog.Successf("Charts synced successfully")
			return nil
		},
	}

	cmd.Flags().StringVar(&syncFromDate, "from-date", "", "Date you want to synchronize charts from. Format: YYYY-MM-DD")
	cmd.Flags().StringVar(&syncWorkdir, "workdir", syncer.DefaultWorkdir(), "Working directory")
	cmd.Flags().BoolVar(&syncLatestVersionOnly, "latest-version-only", false, "Sync only latest version of each chart")
	cmd.Flags().BoolVar(&usePlainHTTP, "use-plain-http", false, "Use plain HTTP instead of HTTPS")
	cmd.Flags().BoolVar(&usePlainLog, "use-plain-log", false, "Use plain klog instead of the pretty logging")

	return cmd
}

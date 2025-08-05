package syncer_test

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/bitnami/charts-syncer/api"
	"github.com/bitnami/charts-syncer/pkg/syncer"
)

func TestFakeSyncPendingCharts(t *testing.T) {
	testCases := []struct {
		desc           string
		entries        []string
		skippedEntries []string
		want           []string
	}{
		{
			desc:    "load apache and kafka",
			entries: []string{"apache", "kafka"},
			want:    []string{"apache-7.3.15.wrap.tgz", "kafka-10.3.3.wrap.tgz"},
		},
		{
			desc:           "skip apache",
			entries:        []string{"apache", "kafka"},
			skippedEntries: []string{"apache"},
			want:           []string{"kafka-10.3.3.wrap.tgz"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			dstTmp, err := os.MkdirTemp("", "charts-syncer-tests-dst-fake")
			if err != nil {
				t.Fatalf("error creating temporary folder: %v", err)
			}
			defer os.RemoveAll(dstTmp)

			s := syncer.NewFake(t, syncer.WithFakeSyncerDestination(dstTmp), syncer.WithFakeSkipCharts(tc.skippedEntries))

			if err := s.SyncPendingCharts(tc.entries...); err != nil {
				t.Error(err)
			}

			gotFiles, err := filepath.Glob(fmt.Sprintf("%s/*.tgz", dstTmp))
			if err != nil {
				t.Fatalf("error listing tgz files: %v", err)
			}

			var got []string
			for _, file := range gotFiles {
				got = append(got, filepath.Base(file))
			}

			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got: %v, want: %v\n", got, tc.want)
			}
		})
	}
}

func TestFakeSyncCherryPickedCharts(t *testing.T) {
	testCases := []struct {
		desc               string
		cherryPickedCharts []*api.CherryPickedChart
		want               []string
	}{
		{
			desc: "sync apache specific version",
			cherryPickedCharts: []*api.CherryPickedChart{
				{
					Name:     "apache",
					Versions: []string{"7.3.15"},
				},
			},
			want: []string{"apache-7.3.15.wrap.tgz"},
		},
		{
			desc: "sync multiple charts with specific versions",
			cherryPickedCharts: []*api.CherryPickedChart{
				{
					Name:     "apache",
					Versions: []string{"7.3.15"},
				},
				{
					Name:     "kafka",
					Versions: []string{"10.3.3"},
				},
			},
			want: []string{"apache-7.3.15.wrap.tgz", "kafka-10.3.3.wrap.tgz"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			dstTmp, err := os.MkdirTemp("", "charts-syncer-tests-dst-fake-cherry")
			if err != nil {
				t.Fatalf("error creating temporary folder: %v", err)
			}
			defer os.RemoveAll(dstTmp)

			s := syncer.NewFake(t,
				syncer.WithFakeSyncerDestination(dstTmp),
				syncer.WithFakeSyncerCherryPickedCharts(tc.cherryPickedCharts),
			)

			// Convert cherry picked charts to regular chart names
			var chartsToSync []string
			for _, cherryChart := range tc.cherryPickedCharts {
				chartsToSync = append(chartsToSync, cherryChart.Name)
			}

			// Use SyncPendingCharts instead of SyncCherryPickedCharts
			if err := s.SyncPendingCharts(chartsToSync...); err != nil {
				t.Error(err)
			}

			gotFiles, err := filepath.Glob(fmt.Sprintf("%s/*.tgz", dstTmp))
			if err != nil {
				t.Fatalf("error listing tgz files: %v", err)
			}

			var got []string
			for _, file := range gotFiles {
				got = append(got, filepath.Base(file))
			}

			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got: %v, want: %v\n", got, tc.want)
			}
		})
	}
}

func TestFakeSyncMixedCharts(t *testing.T) {
	testCases := []struct {
		desc               string
		regularCharts      []string
		cherryPickedCharts []*api.CherryPickedChart
		want               []string
	}{
		{
			desc:          "sync regular and cherry picked charts",
			regularCharts: []string{"kafka"},
			cherryPickedCharts: []*api.CherryPickedChart{
				{
					Name:     "apache",
					Versions: []string{"7.3.15"},
				},
			},
			want: []string{"apache-7.3.15.wrap.tgz", "kafka-10.3.3.wrap.tgz"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			dstTmp, err := os.MkdirTemp("", "charts-syncer-tests-dst-fake-mixed")
			if err != nil {
				t.Fatalf("error creating temporary folder: %v", err)
			}
			defer os.RemoveAll(dstTmp)

			s := syncer.NewFake(t,
				syncer.WithFakeSyncerDestination(dstTmp),
				syncer.WithFakeSyncerCherryPickedCharts(tc.cherryPickedCharts),
			)

			// Combine regular charts and cherry picked charts
			allCharts := make([]string, 0, len(tc.regularCharts)+len(tc.cherryPickedCharts))

			// Add regular charts
			allCharts = append(allCharts, tc.regularCharts...)

			// Add cherry picked chart names
			for _, cherryChart := range tc.cherryPickedCharts {
				allCharts = append(allCharts, cherryChart.Name)
			}

			// Sync all charts together using SyncPendingCharts
			if len(allCharts) > 0 {
				if err := s.SyncPendingCharts(allCharts...); err != nil {
					t.Error(err)
				}
			}

			gotFiles, err := filepath.Glob(fmt.Sprintf("%s/*.tgz", dstTmp))
			if err != nil {
				t.Fatalf("error listing tgz files: %v", err)
			}

			var got []string
			for _, file := range gotFiles {
				got = append(got, filepath.Base(file))
			}

			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got: %v, want: %v\n", got, tc.want)
			}
		})
	}
}

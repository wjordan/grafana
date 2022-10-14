package ualert

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/prometheus/alertmanager/pkg/labels"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/grafana/grafana/pkg/components/simplejson"
	ngModels "github.com/grafana/grafana/pkg/services/ngalert/models"
)

func TestFilterReceiversForAlert(t *testing.T) {
	tc := []struct {
		name             string
		channelIds       []uidOrID
		receivers        map[uidOrID]*PostableApiReceiver
		defaultReceivers map[string]struct{}
		expected         map[string]interface{}
	}{
		{
			name:       "when an alert has multiple channels, each should filter for the correct receiver",
			channelIds: []uidOrID{"uid1", "uid2"},
			receivers: map[uidOrID]*PostableApiReceiver{
				"uid1": {
					Name:                    "recv1",
					GrafanaManagedReceivers: []*PostableGrafanaReceiver{},
				},
				"uid2": {
					Name:                    "recv2",
					GrafanaManagedReceivers: []*PostableGrafanaReceiver{},
				},
				"uid3": {
					Name:                    "recv3",
					GrafanaManagedReceivers: []*PostableGrafanaReceiver{},
				},
			},
			defaultReceivers: map[string]struct{}{},
			expected: map[string]interface{}{
				"recv1": struct{}{},
				"recv2": struct{}{},
			},
		},
		{
			name:       "when default receivers exist, they should be added to an alert's filtered receivers",
			channelIds: []uidOrID{"uid1"},
			receivers: map[uidOrID]*PostableApiReceiver{
				"uid1": {
					Name:                    "recv1",
					GrafanaManagedReceivers: []*PostableGrafanaReceiver{},
				},
				"uid2": {
					Name:                    "recv2",
					GrafanaManagedReceivers: []*PostableGrafanaReceiver{},
				},
				"uid3": {
					Name:                    "recv3",
					GrafanaManagedReceivers: []*PostableGrafanaReceiver{},
				},
			},
			defaultReceivers: map[string]struct{}{
				"recv2": {},
			},
			expected: map[string]interface{}{
				"recv1": struct{}{}, // From alert
				"recv2": struct{}{}, // From default
			},
		},
		{
			name:       "when an alert has a channels associated by ID instead of UID, it should be included",
			channelIds: []uidOrID{int64(42)},
			receivers: map[uidOrID]*PostableApiReceiver{
				int64(42): {
					Name:                    "recv1",
					GrafanaManagedReceivers: []*PostableGrafanaReceiver{},
				},
			},
			defaultReceivers: map[string]struct{}{},
			expected: map[string]interface{}{
				"recv1": struct{}{},
			},
		},
		{
			name:       "when an alert's receivers are covered by the defaults, return nil to use default receiver downstream",
			channelIds: []uidOrID{"uid1"},
			receivers: map[uidOrID]*PostableApiReceiver{
				"uid1": {
					Name:                    "recv1",
					GrafanaManagedReceivers: []*PostableGrafanaReceiver{},
				},
				"uid2": {
					Name:                    "recv2",
					GrafanaManagedReceivers: []*PostableGrafanaReceiver{},
				},
				"uid3": {
					Name:                    "recv3",
					GrafanaManagedReceivers: []*PostableGrafanaReceiver{},
				},
			},
			defaultReceivers: map[string]struct{}{
				"recv1": {},
				"recv2": {},
			},
			expected: nil, // recv1 is already a default
		},
	}

	for _, tt := range tc {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestMigration(t)
			res := m.filterReceiversForAlert("", tt.channelIds, tt.receivers, tt.defaultReceivers)

			require.Equal(t, tt.expected, res)
		})
	}
}

func TestCreateRoute(t *testing.T) {
	tc := []struct {
		name     string
		recv     *PostableApiReceiver
		expected *Route
	}{
		{
			name: "when a receiver is passed in, the route should regex match based on quoted name with continue=true",
			recv: &PostableApiReceiver{
				Name: "recv1",
			},
			expected: &Route{
				Receiver:   "recv1",
				Matchers:   Matchers{{Type: 2, Name: ContactLabel, Value: `.*"recv1".*`}},
				Routes:     nil,
				Continue:   true,
				GroupByStr: nil,
			},
		},
	}

	for _, tt := range tc {
		t.Run(tt.name, func(t *testing.T) {
			res, err := createRoute(tt.recv)
			require.NoError(t, err)

			// Order of nested routes is not guaranteed.
			cOpt := []cmp.Option{
				cmpopts.SortSlices(func(a, b *Route) bool {
					if a.Receiver != b.Receiver {
						return a.Receiver < b.Receiver
					}
					return a.Matchers[0].Value < b.Matchers[0].Value
				}),
				cmpopts.IgnoreUnexported(Route{}, labels.Matcher{}),
			}

			if !cmp.Equal(tt.expected, res, cOpt...) {
				t.Errorf("Unexpected Route: %v", cmp.Diff(tt.expected, res, cOpt...))
			}
		})
	}
}

func createNotChannel(t *testing.T, uid string, id int64, name string) *notificationChannel {
	t.Helper()
	return &notificationChannel{Uid: uid, ID: id, Name: name, Settings: simplejson.New()}
}

func TestCreateReceivers(t *testing.T) {
	tc := []struct {
		name            string
		allChannels     []*notificationChannel
		defaultChannels []*notificationChannel
		expRecvMap      map[uidOrID]*PostableApiReceiver
		expRecv         []*PostableApiReceiver
		expErr          error
	}{
		{
			name:        "when given notification channels migrate them to receivers",
			allChannels: []*notificationChannel{createNotChannel(t, "uid1", int64(1), "name1"), createNotChannel(t, "uid2", int64(2), "name2")},
			expRecvMap: map[uidOrID]*PostableApiReceiver{
				"uid1": {
					Name:                    "name1",
					GrafanaManagedReceivers: []*PostableGrafanaReceiver{{Name: "name1"}},
				},
				"uid2": {
					Name:                    "name2",
					GrafanaManagedReceivers: []*PostableGrafanaReceiver{{Name: "name2"}},
				},
				int64(1): {
					Name:                    "name1",
					GrafanaManagedReceivers: []*PostableGrafanaReceiver{{Name: "name1"}},
				},
				int64(2): {
					Name:                    "name2",
					GrafanaManagedReceivers: []*PostableGrafanaReceiver{{Name: "name2"}},
				},
			},
			expRecv: []*PostableApiReceiver{
				{
					Name:                    "name1",
					GrafanaManagedReceivers: []*PostableGrafanaReceiver{{Name: "name1"}},
				},
				{
					Name:                    "name2",
					GrafanaManagedReceivers: []*PostableGrafanaReceiver{{Name: "name2"}},
				},
			},
		},
	}

	for _, tt := range tc {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestMigration(t)
			recvMap, recvs, err := m.createReceivers(tt.allChannels)
			if tt.expErr != nil {
				require.Error(t, err)
				require.EqualError(t, err, tt.expErr.Error())
				return
			}

			require.NoError(t, err)

			// We ignore certain fields for the purposes of this test
			for _, recv := range recvs {
				for _, not := range recv.GrafanaManagedReceivers {
					not.UID = ""
					not.Settings = nil
					not.SecureSettings = nil
				}
			}

			require.Equal(t, tt.expRecvMap, recvMap)
			require.ElementsMatch(t, tt.expRecv, recvs)
		})
	}
}

func TestCreateDefaultRouteAndReceiver(t *testing.T) {
	tc := []struct {
		name            string
		amConfig        *PostableUserConfig
		defaultChannels []*notificationChannel
		expRecv         *PostableApiReceiver
		expRoute        *Route
		expErr          error
	}{
		{
			name:            "when given multiple default notification channels migrate them to a single receiver",
			defaultChannels: []*notificationChannel{createNotChannel(t, "uid1", int64(1), "name1"), createNotChannel(t, "uid2", int64(2), "name2")},
			expRecv: &PostableApiReceiver{
				Name:                    "autogen-contact-point-default",
				GrafanaManagedReceivers: []*PostableGrafanaReceiver{{Name: "name1"}, {Name: "name2"}},
			},
			expRoute: &Route{
				Receiver:   "autogen-contact-point-default",
				Routes:     make([]*Route, 0),
				GroupByStr: []string{ngModels.FolderTitleLabel, model.AlertNameLabel},
			},
		},
		{
			name:            "when given no default notification channels create a single empty receiver for default",
			defaultChannels: []*notificationChannel{},
			expRecv: &PostableApiReceiver{
				Name:                    "autogen-contact-point-default",
				GrafanaManagedReceivers: []*PostableGrafanaReceiver{},
			},
			expRoute: &Route{
				Receiver:   "autogen-contact-point-default",
				Routes:     make([]*Route, 0),
				GroupByStr: []string{ngModels.FolderTitleLabel, model.AlertNameLabel},
			},
		},
		{
			name:            "when given a single default notification channels don't create a new default receiver",
			defaultChannels: []*notificationChannel{createNotChannel(t, "uid1", int64(1), "name1")},
			expRecv:         nil,
			expRoute: &Route{
				Receiver:   "name1",
				Routes:     make([]*Route, 0),
				GroupByStr: []string{ngModels.FolderTitleLabel, model.AlertNameLabel},
			},
		},
	}

	for _, tt := range tc {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestMigration(t)
			recv, route, err := m.createDefaultRouteAndReceiver(tt.defaultChannels)
			if tt.expErr != nil {
				require.Error(t, err)
				require.EqualError(t, err, tt.expErr.Error())
				return
			}

			require.NoError(t, err)

			// We ignore certain fields for the purposes of this test
			if recv != nil {
				for _, not := range recv.GrafanaManagedReceivers {
					not.UID = ""
					not.Settings = nil
					not.SecureSettings = nil
				}
			}

			require.Equal(t, tt.expRecv, recv)
			require.Equal(t, tt.expRoute, route)
		})
	}
}

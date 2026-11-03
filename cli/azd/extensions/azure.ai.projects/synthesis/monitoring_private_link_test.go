// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package synthesis

import (
	"errors"
	"strings"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testVNetID = "/subscriptions/" + testSub + "/resourceGroups/net/providers/Microsoft.Network/virtualNetworks/vnet"
	testPEID   = "/subscriptions/" + testSub + "/resourceGroups/net/providers/Microsoft.Network/privateEndpoints/pe"
)

func privateLinkReader(group, scopeID string) *fakeReader {
	connection := func(groupID string) []any {
		return []any{map[string]any{"properties": map[string]any{
			"groupIds": []any{groupID}, "privateLinkServiceId": scopeID,
		}}}
	}
	return &fakeReader{resources: map[string]*armresources.GenericResource{
		strings.ToLower(testVNetID): armResource(testVNetID, map[string]any{"subnets": []any{
			map[string]any{"properties": map[string]any{"privateEndpoints": []any{map[string]any{"id": testPEID}}}},
		}}),
		strings.ToLower(testPEID): armResource(testPEID, map[string]any{
			"privateLinkServiceConnections": connection(group),
		}),
	}}
}

func TestCheckMonitorPrivateLinkConflict(t *testing.T) {
	t.Parallel()
	const ownedScope = "/subscriptions/" + testSub + "/resourceGroups/rg/providers/Microsoft.Insights/privateLinkScopes/ampls-abc"
	const foreignScope = "/subscriptions/" + testSub + "/resourceGroups/shared/providers/Microsoft.Insights/privateLinkScopes/corp"
	const sameGroupForeign = "/subscriptions/" + testSub + "/resourceGroups/rg/providers/Microsoft.Insights/privateLinkScopes/corp"

	tests := []struct {
		name     string
		reader   *fakeReader
		conflict bool
		err      bool
	}{
		{name: "no private endpoints", reader: &fakeReader{resources: map[string]*armresources.GenericResource{
			strings.ToLower(testVNetID): armResource(testVNetID, map[string]any{}),
		}}},
		{name: "virtual network does not exist yet", reader: &fakeReader{}},
		{name: "unrelated private endpoint", reader: privateLinkReader("blob", foreignScope)},
		{name: "owned scope from a previous run", reader: privateLinkReader("azuremonitor", ownedScope)},
		{name: "foreign scope", reader: privateLinkReader("azuremonitor", foreignScope), conflict: true},
		{name: "foreign scope in the deployment group", reader: privateLinkReader("azuremonitor", sameGroupForeign), conflict: true},
		{name: "read failure", reader: &fakeReader{resourceErr: errors.New("boom")}, err: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := (&MonitoringResolver{Reader: tt.reader}).CheckMonitorPrivateLinkConflict(t.Context(), testVNetID, "rg")
			var conflict *MonitorPrivateLinkConflictError
			switch {
			case tt.conflict:
				require.ErrorAs(t, err, &conflict)
				assert.Equal(t, testPEID, conflict.PrivateEndpointID)
				assert.Contains(t, err.Error(), "monitoring.enabled: false")
			case tt.err:
				require.Error(t, err)
				assert.False(t, errors.As(err, &conflict))
			default:
				require.NoError(t, err)
			}
		})
	}
}

func TestMonitoringKeyPolicyForPrivateNetwork(t *testing.T) {
	t.Parallel()
	disabled := map[string]any{"publicNetworkAccessForIngestion": "Disabled"}
	assert.Error(t, MonitoringKeyPolicyFor(disabled, false))
	assert.NoError(t, MonitoringKeyPolicyFor(disabled, true))
	assert.Error(t, MonitoringKeyPolicyFor(map[string]any{"DisableLocalAuth": true}, true))
}

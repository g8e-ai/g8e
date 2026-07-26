// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package lattice

import (
	"context"
	"fmt"
	"time"

	entitymanagerv1 "github.com/g8e-ai/g8e/internal/adapters/lattice/gen/anduril/entitymanager/v1"
	"github.com/g8e-ai/g8e/internal/constants"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// presenceExpiryDuration is the soft-state TTL for entity presence in
// Lattice's COP. The heartbeat interval must be shorter than this to
// guarantee republish within the expiry window.
const presenceExpiryDuration = 5 * time.Minute

// PublishPresence publishes or updates the g8e entity in Lattice's COP.
// Uses retryWithBackoff for transient failures. Returns
// ErrLatticePresencePublishFailed on persistent failure.
func (a *Adapter) PublishPresence(ctx context.Context) error {
	expiry := time.Now().Add(presenceExpiryDuration)

	entity := &entitymanagerv1.Entity{
		EntityId:   a.entityID,
		IsLive:     true,
		ExpiryTime: timestamppb.New(expiry),
		Aliases: &entitymanagerv1.Aliases{
			Name: a.config.Entity.Name,
		},
		Provenance: &entitymanagerv1.Provenance{
			IntegrationName:  "g8e",
			DataType:         "g8e-operator",
			SourceUpdateTime: timestamppb.Now(),
		},
		Ontology: &entitymanagerv1.Ontology{
			PlatformType: a.config.Entity.PlatformType,
			Template:     entitymanagerv1.Template_TEMPLATE_ASSET,
		},
	}

	if a.config.Entity.Latitude != 0 || a.config.Entity.Longitude != 0 {
		entity.Location = &entitymanagerv1.Location{
			Position: &entitymanagerv1.Position{
				LatitudeDegrees:  a.config.Entity.Latitude,
				LongitudeDegrees: a.config.Entity.Longitude,
			},
		}
	}

	req := &entitymanagerv1.PublishEntityRequest{
		Entity: entity,
	}

	if _, err := a.entityMgr.PublishEntity(ctx, req); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrLatticePresencePublishFailed, err)
	}

	return nil
}

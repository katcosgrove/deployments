// Copyright 2019 Northern.tech AS
//
//    Licensed under the Apache License, Version 2.0 (the "License");
//    you may not use this file except in compliance with the License.
//    You may obtain a copy of the License at
//
//        http://www.apache.org/licenses/LICENSE-2.0
//
//    Unless required by applicable law or agreed to in writing, software
//    distributed under the License is distributed on an "AS IS" BASIS,
//    WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//    See the License for the specific language governing permissions and
//    limitations under the License.
package store

import (
	"context"
	"time"

	"github.com/mendersoftware/deployments/v2/model"
)

type DataStore interface {
	//releases
	GetReleases(ctx context.Context, filt *model.ReleaseFilter) ([]model.Release, error)

	//limits
	GetLimit(ctx context.Context, name string) (*model.Limit, error)

	//tenants
	ProvisionTenant(ctx context.Context, tenantId string) error

	//images
	Exists(ctx context.Context, id string) (bool, error)
	Update(ctx context.Context, image *model.SoftwareImage) (bool, error)
	InsertImage(ctx context.Context, image *model.SoftwareImage) error
	FindImageByID(ctx context.Context, id string) (*model.SoftwareImage, error)
	IsArtifactUnique(ctx context.Context, artifactName string,
		deviceTypesCompatible []string) (bool, error)
	DeleteImage(ctx context.Context, id string) error
	FindAll(ctx context.Context) ([]*model.SoftwareImage, error)

	//artifact getter
	ImagesByName(ctx context.Context,
		artifactName string) ([]*model.SoftwareImage, error)
	ImageByIdsAndDeviceType(ctx context.Context,
		ids []string, deviceType string) (*model.SoftwareImage, error)
	ImageByNameAndDeviceType(ctx context.Context,
		name, deviceType string) (*model.SoftwareImage, error)

	//device deployment log
	SaveDeviceDeploymentLog(ctx context.Context, log model.DeploymentLog) error
	GetDeviceDeploymentLog(ctx context.Context,
		deviceID, deploymentID string) (*model.DeploymentLog, error)

	// device deployemnts
	InsertMany(ctx context.Context,
		deployment ...*model.DeviceDeployment) error
	ExistAssignedImageWithIDAndStatuses(ctx context.Context,
		id string, statuses ...string) (bool, error)
	FindOldestDeploymentForDeviceIDWithStatuses(ctx context.Context,
		deviceID string, statuses ...string) (*model.DeviceDeployment, error)
	FindAllDeploymentsForDeviceIDWithStatuses(ctx context.Context,
		deviceID string, statuses ...string) ([]model.DeviceDeployment, error)
	UpdateDeviceDeploymentStatus(ctx context.Context, deviceID string,
		deploymentID string, status model.DeviceDeploymentStatus) (string, error)
	UpdateDeviceDeploymentLogAvailability(ctx context.Context,
		deviceID string, deploymentID string, log bool) error
	AssignArtifact(ctx context.Context, deviceID string,
		deploymentID string, artifact *model.SoftwareImage) error
	AggregateDeviceDeploymentByStatus(ctx context.Context,
		id string) (model.Stats, error)
	GetDeviceStatusesForDeployment(ctx context.Context,
		deploymentID string) ([]model.DeviceDeployment, error)
	HasDeploymentForDevice(ctx context.Context,
		deploymentID string, deviceID string) (bool, error)
	GetDeviceDeploymentStatus(ctx context.Context,
		deploymentID string, deviceID string) (string, error)
	AbortDeviceDeployments(ctx context.Context, deploymentID string) error
	DecommissionDeviceDeployments(ctx context.Context, deviceId string) error

	// deployments
	InsertDeployment(ctx context.Context, deployment *model.Deployment) error
	DeleteDeployment(ctx context.Context, id string) error
	FindDeploymentByID(ctx context.Context, id string) (*model.Deployment, error)
	FindUnfinishedByID(ctx context.Context,
		id string) (*model.Deployment, error)
	UpdateStats(ctx context.Context, id string, state_from, state_to string) error
	UpdateStatsAndFinishDeployment(ctx context.Context,
		id string, stats model.Stats) error
	Find(ctx context.Context,
		query model.Query) ([]*model.Deployment, error)
	Finish(ctx context.Context, id string, when time.Time) error
	ExistUnfinishedByArtifactId(ctx context.Context, id string) (bool, error)
	ExistByArtifactId(ctx context.Context, id string) (bool, error)
	DeviceCountByDeployment(ctx context.Context, id string) (int, error)
}

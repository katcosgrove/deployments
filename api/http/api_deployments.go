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
package http

import (
	"fmt"
	"io"
	"io/ioutil"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/ant0ine/go-json-rest/rest"
	"github.com/asaskevich/govalidator"
	"github.com/pkg/errors"

	"github.com/mendersoftware/go-lib-micro/identity"
	"github.com/mendersoftware/go-lib-micro/requestlog"
	"github.com/mendersoftware/go-lib-micro/rest_utils"

	"github.com/mendersoftware/deployments/v2/app"
	"github.com/mendersoftware/deployments/v2/model"
	"github.com/mendersoftware/deployments/v2/store"
)

const (
	// 15 minutes
	DefaultDownloadLinkExpire = 15 * time.Minute

	DefaultMaxMetaSize = 1024 * 1024 * 10
)

// storage keys
const (
	GetDeploymentForDeviceQueryArtifact   = "artifact_name"
	GetDeploymentForDeviceQueryDeviceType = "device_type"
)

// Errors
var (
	ErrIDNotUUIDv4                    = errors.New("ID is not UUIDv4")
	ErrArtifactUsedInActiveDeployment = errors.New("Artifact is used in active deployment")
	ErrInvalidExpireParam             = errors.New("Invalid expire parameter")

	ErrInternal                   = errors.New("Internal error")
	ErrDeploymentAlreadyFinished  = errors.New("Deployment already finished")
	ErrUnexpectedDeploymentStatus = errors.New("Unexpected deployment status")
	ErrMissingIdentity            = errors.New("Missing identity data")
)

type DeploymentsApiHandlers struct {
	view  RESTView
	store store.DataStore
	app   app.App
}

func NewDeploymentsApiHandlers(store store.DataStore, view RESTView, app app.App) *DeploymentsApiHandlers {
	return &DeploymentsApiHandlers{
		store: store,
		view:  view,
		app:   app,
	}
}

func (d *DeploymentsApiHandlers) GetReleases(w rest.ResponseWriter, r *rest.Request) {
	l := requestlog.GetRequestLogger(r)

	var filt *model.ReleaseFilter

	name := r.URL.Query().Get("name")

	if name != "" {
		filt = &model.ReleaseFilter{
			Name: name,
		}
	}

	releases, err := d.store.GetReleases(r.Context(), filt)
	if err != nil {
		d.view.RenderInternalError(w, r, err, l)
		return
	}

	d.view.RenderSuccessGet(w, releases)
}

type limitResponse struct {
	Limit uint64 `json:"limit"`
	Usage uint64 `json:"usage"`
}

func (d *DeploymentsApiHandlers) GetLimit(w rest.ResponseWriter, r *rest.Request) {
	l := requestlog.GetRequestLogger(r)

	name := r.PathParam("name")

	if !model.IsValidLimit(name) {
		d.view.RenderError(w, r,
			errors.Errorf("unsupported limit %s", name),
			http.StatusBadRequest, l)
		return
	}

	limit, err := d.app.GetLimit(r.Context(), name)
	if err != nil {
		d.view.RenderInternalError(w, r, err, l)
		return
	}

	d.view.RenderSuccessGet(w, limitResponse{
		Limit: limit.Value,
		Usage: 0, // TODO fill this when ready
	})
}

// images

func (d *DeploymentsApiHandlers) GetImage(w rest.ResponseWriter, r *rest.Request) {
	l := requestlog.GetRequestLogger(r)

	id := r.PathParam("id")

	if !govalidator.IsUUIDv4(id) {
		d.view.RenderError(w, r, ErrIDNotUUIDv4, http.StatusBadRequest, l)
		return
	}

	image, err := d.app.GetImage(r.Context(), id)
	if err != nil {
		d.view.RenderInternalError(w, r, err, l)
		return
	}

	if image == nil {
		d.view.RenderErrorNotFound(w, r, l)
		return
	}

	d.view.RenderSuccessGet(w, image)
}

func (d *DeploymentsApiHandlers) ListImages(w rest.ResponseWriter, r *rest.Request) {
	l := requestlog.GetRequestLogger(r)

	list, err := d.app.ListImages(r.Context(), r.PathParams)
	if err != nil {
		d.view.RenderInternalError(w, r, err, l)
		return
	}

	d.view.RenderSuccessGet(w, list)
}

func (d *DeploymentsApiHandlers) DownloadLink(w rest.ResponseWriter, r *rest.Request) {
	l := requestlog.GetRequestLogger(r)

	id := r.PathParam("id")

	if !govalidator.IsUUIDv4(id) {
		d.view.RenderError(w, r, ErrIDNotUUIDv4, http.StatusBadRequest, l)
		return
	}

	link, err := d.app.DownloadLink(r.Context(), id, DefaultDownloadLinkExpire)
	if err != nil {
		d.view.RenderInternalError(w, r, err, l)
		return
	}

	if link == nil {
		d.view.RenderErrorNotFound(w, r, l)
		return
	}

	d.view.RenderSuccessGet(w, link)
}

func (d *DeploymentsApiHandlers) DeleteImage(w rest.ResponseWriter, r *rest.Request) {
	l := requestlog.GetRequestLogger(r)

	id := r.PathParam("id")

	if !govalidator.IsUUIDv4(id) {
		d.view.RenderError(w, r, ErrIDNotUUIDv4, http.StatusBadRequest, l)
		return
	}

	if err := d.app.DeleteImage(r.Context(), id); err != nil {
		switch err {
		default:
			d.view.RenderInternalError(w, r, err, l)
		case app.ErrImageMetaNotFound:
			d.view.RenderErrorNotFound(w, r, l)
		case app.ErrModelImageInActiveDeployment:
			d.view.RenderError(w, r, ErrArtifactUsedInActiveDeployment, http.StatusConflict, l)
		}
		return
	}

	d.view.RenderSuccessDelete(w)
}

func (d *DeploymentsApiHandlers) EditImage(w rest.ResponseWriter, r *rest.Request) {
	l := requestlog.GetRequestLogger(r)

	id := r.PathParam("id")

	if !govalidator.IsUUIDv4(id) {
		d.view.RenderError(w, r, ErrIDNotUUIDv4, http.StatusBadRequest, l)
		return
	}

	constructor, err := getSoftwareImageMetaConstructorFromBody(r)
	if err != nil {
		d.view.RenderError(w, r, errors.Wrap(err, "Validating request body"), http.StatusBadRequest, l)
		return
	}

	found, err := d.app.EditImage(r.Context(), id, constructor)
	if err != nil {
		if err == app.ErrModelImageUsedInAnyDeployment {
			d.view.RenderError(w, r, err, http.StatusUnprocessableEntity, l)
			return
		}
		d.view.RenderInternalError(w, r, err, l)
		return
	}

	if !found {
		d.view.RenderErrorNotFound(w, r, l)
		return
	}

	d.view.RenderSuccessPut(w)
}

func getSoftwareImageMetaConstructorFromBody(r *rest.Request) (*model.SoftwareImageMetaConstructor, error) {

	var constructor *model.SoftwareImageMetaConstructor

	if err := r.DecodeJsonPayload(&constructor); err != nil {
		return nil, err
	}

	if err := constructor.Validate(); err != nil {
		return nil, err
	}

	return constructor, nil
}

// Multipart Image/Meta upload handler.
// Request should be of type "multipart/form-data".
// First part should contain Metadata file. This file should be of type "application/json".
// Second part should contain artifact file.
func (d *DeploymentsApiHandlers) NewImage(w rest.ResponseWriter, r *rest.Request) {
	l := requestlog.GetRequestLogger(r)

	// parse content type and params according to RFC 1521
	_, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil {
		d.view.RenderError(w, r, err, http.StatusBadRequest, l)
		return
	}

	mr := multipart.NewReader(r.Body, params["boundary"])
	// parse multipart message
	multipartUploadMsg, err := d.ParseMultipart(mr, DefaultMaxMetaSize)
	if err != nil {
		d.view.RenderError(w, r, err, http.StatusBadRequest, l)
		return
	}

	imgID, err := d.app.CreateImage(r.Context(), multipartUploadMsg)
	cause := errors.Cause(err)
	switch cause {
	default:
		d.view.RenderInternalError(w, r, err, l)
	case nil:
		d.view.RenderSuccessPost(w, r, imgID)
	case app.ErrModelArtifactNotUnique:
		l.Error(err.Error())
		d.view.RenderError(w, r, cause, http.StatusUnprocessableEntity, l)
	case app.ErrModelParsingArtifactFailed:
		l.Error(err.Error())
		d.view.RenderError(w, r, formatArtifactUploadError(err), http.StatusBadRequest, l)
	case app.ErrModelMissingInputMetadata, app.ErrModelMissingInputArtifact,
		app.ErrModelInvalidMetadata, app.ErrModelMultipartUploadMsgMalformed,
		app.ErrModelArtifactFileTooLarge:
		l.Error(err.Error())
		d.view.RenderError(w, r, cause, http.StatusBadRequest, l)
	}

	return
}

func formatArtifactUploadError(err error) error {
	// remove generic message
	errMsg := strings.TrimSuffix(err.Error(), ": "+app.ErrModelParsingArtifactFailed.Error())

	// handle specific cases

	if strings.Contains(errMsg, "invalid checksum") {
		return errors.New(errMsg[strings.Index(errMsg, "invalid checksum"):])
	}

	if strings.Contains(errMsg, "unsupported version") {
		return errors.New(errMsg[strings.Index(errMsg, "unsupported version"):] +
			"; supported versions are: 1, 2")
	}

	return errors.New(errMsg)
}

// ParseMultipart parses multipart/form-data message.
func (d *DeploymentsApiHandlers) ParseMultipart(mr *multipart.Reader, maxMetaSize int64) (*model.MultipartUploadMsg, error) {
	multipartUploadMsg := &model.MultipartUploadMsg{
		MetaConstructor: &model.SoftwareImageMetaConstructor{},
	}
	for {
		p, err := mr.NextPart()
		if err != nil {
			return nil, errors.Wrap(err, "Request does not contain artifact")
		}
		switch p.FormName() {
		case "size":
			size, err := d.getFormFieldValue(p, maxMetaSize)
			if err != nil {
				return nil, err
			}
			multipartUploadMsg.ArtifactSize, err = strconv.ParseInt(*size, 10, 64)
			if err != nil {
				return nil, err
			}
		case "description":
			desc, err := d.getFormFieldValue(p, maxMetaSize)
			if err != nil {
				return nil, err
			}
			multipartUploadMsg.MetaConstructor.Description = *desc
		case "artifact":
			// valide metadata provided by the user and the image size
			if err := multipartUploadMsg.MetaConstructor.Validate(); err != nil {
				return nil, err
			}
			// artifact size part should be provided before artifact part
			// artifact size value should be greater then 0
			if multipartUploadMsg.ArtifactSize <= 0 {
				return nil, errors.New("No size provided before the artifact part of the message or the size value is wrong.")
			}
			// HTML form can't set specific content-type, it's automatic, if not empty - it's a file
			if p.Header.Get("Content-Type") == "" {
				return nil, errors.New("The last part of the multipart/form-data message should be an artifact.")
			}
			multipartUploadMsg.ArtifactReader = p
			return multipartUploadMsg, nil
		}
	}
}

func (d *DeploymentsApiHandlers) getFormFieldValue(p *multipart.Part, maxMetaSize int64) (*string, error) {
	metaReader := io.LimitReader(p, maxMetaSize)
	bytes, err := ioutil.ReadAll(metaReader)
	if err != nil && err != io.EOF {
		return nil, errors.Wrap(err, "Failed to obtain value for "+p.FormName())
	}

	strValue := string(bytes)
	return &strValue, nil
}

// deployments

func (d *DeploymentsApiHandlers) PostDeployment(w rest.ResponseWriter, r *rest.Request) {
	ctx := r.Context()
	l := requestlog.GetRequestLogger(r)

	constructor, err := d.getDeploymentConstructorFromBody(r)
	if err != nil {
		d.view.RenderError(w, r, errors.Wrap(err, "Validating request body"), http.StatusBadRequest, l)
		return
	}

	id, err := d.app.CreateDeployment(ctx, constructor)
	if err != nil {
		if err == app.ErrNoArtifact {
			d.view.RenderError(w, r, err, http.StatusUnprocessableEntity, l)
		} else {
			d.view.RenderInternalError(w, r, err, l)
		}
		return
	}

	d.view.RenderSuccessPost(w, r, id)
}

func (d *DeploymentsApiHandlers) getDeploymentConstructorFromBody(r *rest.Request) (*model.DeploymentConstructor, error) {
	var constructor *model.DeploymentConstructor
	if err := r.DecodeJsonPayload(&constructor); err != nil {
		return nil, err
	}

	if err := constructor.Validate(); err != nil {
		return nil, err
	}

	return constructor, nil
}

func (d *DeploymentsApiHandlers) GetDeployment(w rest.ResponseWriter, r *rest.Request) {
	ctx := r.Context()
	l := requestlog.GetRequestLogger(r)

	id := r.PathParam("id")

	if !govalidator.IsUUIDv4(id) {
		d.view.RenderError(w, r, ErrIDNotUUIDv4, http.StatusBadRequest, l)
		return
	}

	deployment, err := d.app.GetDeployment(ctx, id)
	if err != nil {
		d.view.RenderInternalError(w, r, err, l)
		return
	}

	if deployment == nil {
		d.view.RenderErrorNotFound(w, r, l)
		return
	}

	d.view.RenderSuccessGet(w, deployment)
}

func (d *DeploymentsApiHandlers) GetDeploymentStats(w rest.ResponseWriter, r *rest.Request) {
	ctx := r.Context()
	l := requestlog.GetRequestLogger(r)

	id := r.PathParam("id")

	if !govalidator.IsUUIDv4(id) {
		d.view.RenderError(w, r, ErrIDNotUUIDv4, http.StatusBadRequest, l)
		return
	}

	stats, err := d.app.GetDeploymentStats(ctx, id)
	if err != nil {
		d.view.RenderInternalError(w, r, err, l)
		return
	}

	if stats == nil {
		d.view.RenderErrorNotFound(w, r, l)
		return
	}

	d.view.RenderSuccessGet(w, stats)
}

func (d *DeploymentsApiHandlers) AbortDeployment(w rest.ResponseWriter, r *rest.Request) {
	ctx := r.Context()
	l := requestlog.GetRequestLogger(r)

	id := r.PathParam("id")

	if !govalidator.IsUUIDv4(id) {
		d.view.RenderError(w, r, ErrIDNotUUIDv4, http.StatusBadRequest, l)
		return
	}

	// receive request body
	var status struct {
		Status string
	}

	err := r.DecodeJsonPayload(&status)
	if err != nil {
		d.view.RenderError(w, r, err, http.StatusBadRequest, l)
		return
	}
	// "aborted" is the only supported status
	if status.Status != model.DeviceDeploymentStatusAborted {
		d.view.RenderError(w, r, ErrUnexpectedDeploymentStatus, http.StatusBadRequest, l)
	}

	l.Infof("Abort deployment: %s", id)

	// Check if deployment is finished
	isDeploymentFinished, err := d.app.IsDeploymentFinished(ctx, id)
	if err != nil {
		d.view.RenderInternalError(w, r, err, l)
		return
	}
	if isDeploymentFinished {
		d.view.RenderError(w, r, ErrDeploymentAlreadyFinished, http.StatusUnprocessableEntity, l)
		return
	}

	// Abort deployments for devices and update deployment stats
	if err := d.app.AbortDeployment(ctx, id); err != nil {
		d.view.RenderInternalError(w, r, err, l)
	}

	d.view.RenderEmptySuccessResponse(w)
}

func (d *DeploymentsApiHandlers) GetDeploymentForDevice(w rest.ResponseWriter, r *rest.Request) {
	ctx := r.Context()
	l := requestlog.GetRequestLogger(r)

	idata := identity.FromContext(ctx)
	if idata == nil {
		d.view.RenderError(w, r, ErrMissingIdentity, http.StatusBadRequest, l)
		return
	}

	q := r.URL.Query()
	installed := model.InstalledDeviceDeployment{
		Artifact:   q.Get(GetDeploymentForDeviceQueryArtifact),
		DeviceType: q.Get(GetDeploymentForDeviceQueryDeviceType),
	}

	if err := installed.Validate(); err != nil {
		d.view.RenderError(w, r, err, http.StatusBadRequest, l)
		return
	}

	deployment, err := d.app.GetDeploymentForDeviceWithCurrent(ctx, idata.Subject, installed)
	if err != nil {
		d.view.RenderInternalError(w, r, err, l)
		return
	}

	if deployment == nil {
		d.view.RenderNoUpdateForDevice(w)
		return
	}

	d.view.RenderSuccessGet(w, deployment)
}

func (d *DeploymentsApiHandlers) PutDeploymentStatusForDevice(w rest.ResponseWriter, r *rest.Request) {
	ctx := r.Context()
	l := requestlog.GetRequestLogger(r)

	did := r.PathParam("id")

	idata := identity.FromContext(ctx)
	if idata == nil {
		d.view.RenderError(w, r, ErrMissingIdentity, http.StatusBadRequest, l)
		return
	}

	// receive request body
	var report model.StatusReport

	err := r.DecodeJsonPayload(&report)
	if err != nil {
		d.view.RenderError(w, r, err, http.StatusBadRequest, l)
		return
	}

	l.Infof("status: %+v", report)
	if err := d.app.UpdateDeviceDeploymentStatus(ctx, did,
		idata.Subject, model.DeviceDeploymentStatus{
			Status:   report.Status,
			SubState: report.SubState,
		}); err != nil {

		if err == app.ErrDeploymentAborted || err == app.ErrDeviceDecommissioned {
			d.view.RenderError(w, r, err, http.StatusConflict, l)
		} else {
			d.view.RenderInternalError(w, r, err, l)
		}
		return
	}

	d.view.RenderEmptySuccessResponse(w)
}

func (d *DeploymentsApiHandlers) GetDeviceStatusesForDeployment(w rest.ResponseWriter, r *rest.Request) {
	ctx := r.Context()
	l := requestlog.GetRequestLogger(r)

	did := r.PathParam("id")

	if !govalidator.IsUUIDv4(did) {
		d.view.RenderError(w, r, ErrIDNotUUIDv4, http.StatusBadRequest, l)
		return
	}

	statuses, err := d.app.GetDeviceStatusesForDeployment(ctx, did)
	if err != nil {
		switch err {
		case app.ErrModelDeploymentNotFound:
			d.view.RenderError(w, r, err, http.StatusNotFound, l)
			return
		default:
			d.view.RenderInternalError(w, r, ErrInternal, l)
			return
		}
	}

	d.view.RenderSuccessGet(w, statuses)
}

func ParseLookupQuery(vals url.Values) (model.Query, error) {
	query := model.Query{}

	search := vals.Get("search")
	if search != "" {
		query.SearchText = search
	}

	createdBefore := vals.Get("created_before")
	if createdBefore != "" {
		if createdBeforeTime, err := parseEpochToTimestamp(createdBefore); err != nil {
			return query, errors.Wrap(err, "timestamp parsing failed for created_before parameter")
		} else {
			query.CreatedBefore = &createdBeforeTime
		}
	}

	createdAfter := vals.Get("created_after")
	if createdAfter != "" {
		if createdAfterTime, err := parseEpochToTimestamp(createdAfter); err != nil {
			return query, errors.Wrap(err, "timestamp parsing failed created_after parameter")
		} else {
			query.CreatedAfter = &createdAfterTime
		}
	}

	status := vals.Get("status")
	switch status {
	case "inprogress":
		query.Status = model.StatusQueryInProgress
	case "finished":
		query.Status = model.StatusQueryFinished
	case "pending":
		query.Status = model.StatusQueryPending
	case "aborted":
		query.Status = model.StatusQueryAborted
	case "":
		query.Status = model.StatusQueryAny
	default:
		return query, errors.Errorf("unknown status %s", status)

	}

	return query, nil
}

func parseEpochToTimestamp(epoch string) (time.Time, error) {
	if epochInt64, err := strconv.ParseInt(epoch, 10, 64); err != nil {
		return time.Time{}, errors.Errorf("invalid timestamp: " + epoch)
	} else {
		return time.Unix(epochInt64, 0).UTC(), nil
	}
}

func (d *DeploymentsApiHandlers) LookupDeployment(w rest.ResponseWriter, r *rest.Request) {
	ctx := r.Context()
	l := requestlog.GetRequestLogger(r)

	query, err := ParseLookupQuery(r.URL.Query())
	if err != nil {
		d.view.RenderError(w, r, err, http.StatusBadRequest, l)
		return
	}

	page, perPage, err := rest_utils.ParsePagination(r)
	if err != nil {
		d.view.RenderError(w, r, err, http.StatusBadRequest, l)
		return
	}
	query.Skip = int((page - 1) * perPage)
	query.Limit = int(perPage + 1)

	deps, err := d.app.LookupDeployment(ctx, query)
	if err != nil {
		d.view.RenderError(w, r, err, http.StatusBadRequest, l)
		return
	}

	len := len(deps)
	hasNext := false
	if uint64(len) > perPage {
		hasNext = true
		len = int(perPage)
	}

	links := rest_utils.MakePageLinkHdrs(r, page, perPage, hasNext)
	for _, l := range links {
		w.Header().Add("Link", l)
	}

	d.view.RenderSuccessGet(w, deps[:len])
}

func (d *DeploymentsApiHandlers) PutDeploymentLogForDevice(w rest.ResponseWriter, r *rest.Request) {
	ctx := r.Context()
	l := requestlog.GetRequestLogger(r)

	did := r.PathParam("id")

	idata := identity.FromContext(ctx)
	if idata == nil {
		d.view.RenderError(w, r, ErrMissingIdentity, http.StatusBadRequest, l)
		return
	}

	// reuse DeploymentLog, device and deployment IDs are ignored when
	// (un-)marshalling DeploymentLog to/from JSON
	var log model.DeploymentLog

	err := r.DecodeJsonPayload(&log)
	if err != nil {
		d.view.RenderError(w, r, err, http.StatusBadRequest, l)
		return
	}

	if err := d.app.SaveDeviceDeploymentLog(ctx, idata.Subject,
		did, log.Messages); err != nil {

		if err == app.ErrModelDeploymentNotFound {
			d.view.RenderError(w, r, err, http.StatusNotFound, l)
		} else {
			d.view.RenderInternalError(w, r, err, l)
		}
		return
	}

	d.view.RenderEmptySuccessResponse(w)
}

func (d *DeploymentsApiHandlers) GetDeploymentLogForDevice(w rest.ResponseWriter, r *rest.Request) {
	ctx := r.Context()
	l := requestlog.GetRequestLogger(r)

	did := r.PathParam("id")
	devid := r.PathParam("devid")

	depl, err := d.app.GetDeviceDeploymentLog(ctx, devid, did)

	if err != nil {
		d.view.RenderInternalError(w, r, err, l)
		return
	}

	if depl == nil {
		d.view.RenderErrorNotFound(w, r, l)
		return
	}

	d.view.RenderDeploymentLog(w, *depl)
}

func (d *DeploymentsApiHandlers) DecommissionDevice(w rest.ResponseWriter, r *rest.Request) {
	ctx := r.Context()
	l := requestlog.GetRequestLogger(r)

	id := r.PathParam("id")

	// Decommission deployments for devices and update deployment stats
	err := d.app.DecommissionDevice(ctx, id)

	switch err {
	case nil, app.ErrStorageNotFound:
		d.view.RenderEmptySuccessResponse(w)
	default:
		d.view.RenderInternalError(w, r, err, l)

	}
}

// tenants

func (d *DeploymentsApiHandlers) ProvisionTenantsHandler(w rest.ResponseWriter, r *rest.Request) {
	ctx := r.Context()
	l := requestlog.GetRequestLogger(r)

	defer r.Body.Close()

	tenant, err := model.ParseNewTenantReq(r.Body)
	if err != nil {
		rest_utils.RestErrWithLog(w, r, l, err, http.StatusBadRequest)
		return
	}

	err = d.app.ProvisionTenant(ctx, tenant.TenantId)
	if err != nil {
		rest_utils.RestErrWithLogInternal(w, r, l, err)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

func (d *DeploymentsApiHandlers) DeploymentsPerTenantHandler(w rest.ResponseWriter, r *rest.Request) {
	ctx := r.Context()
	l := requestlog.GetRequestLogger(r)
	defer r.Body.Close()

	tenantID := r.PathParam("tenant")

	if tenantID == "" {
		rest_utils.RestErrWithLog(w, r, l, nil, http.StatusBadRequest)
	}

	query, err := ParseLookupQuery(r.URL.Query())

	if err != nil {
		rest_utils.RestErrWithLog(w, r, l, err, http.StatusBadRequest)
	}

	ident := &identity.Identity{Tenant: tenantID}
	ctx = identity.WithContext(r.Context(), ident)

	if deps, err := d.app.LookupDeployment(ctx, query); err != nil {
		rest_utils.RestErrWithLog(w, r, l, err, http.StatusBadRequest)
	} else {
		w.WriteJson(deps)
	}
}

func (d *DeploymentsApiHandlers) NewImageForTenantHandler(w rest.ResponseWriter, r *rest.Request) {
	l := requestlog.GetRequestLogger(r)

	tenantID := r.PathParam("tenant")

	if tenantID == "" {
		rest_utils.RestErrWithLog(w, r, l, fmt.Errorf("missing tenant id in path"), http.StatusBadRequest)
		return
	}

	ident := &identity.Identity{Tenant: tenantID}
	ctx := identity.WithContext(r.Context(), ident)

	// parse content type and params according to RFC 1521
	_, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil {
		rest_utils.RestErrWithLog(w, r, l, err, http.StatusBadRequest)
		return
	}

	mr := multipart.NewReader(r.Body, params["boundary"])
	// parse multipart message

	multipartUploadMsg, err := d.ParseMultipart(mr, DefaultMaxMetaSize)
	if err != nil {
		d.view.RenderError(w, r, err, http.StatusBadRequest, l)
		return
	}

	imgID, err := d.app.CreateImage(ctx, multipartUploadMsg)
	cause := errors.Cause(err)
	switch cause {
	default:
		d.view.RenderInternalError(w, r, err, l)
	case nil:
		d.view.RenderSuccessPost(w, r, imgID)
	case app.ErrModelArtifactNotUnique:
		l.Error(err.Error())
		d.view.RenderError(w, r, cause, http.StatusUnprocessableEntity, l)
	case app.ErrModelMissingInputMetadata, app.ErrModelMissingInputArtifact,
		app.ErrModelInvalidMetadata, app.ErrModelMultipartUploadMsgMalformed,
		app.ErrModelArtifactFileTooLarge, app.ErrModelParsingArtifactFailed:
		l.Error(err.Error())
		d.view.RenderError(w, r, cause, http.StatusBadRequest, l)
	}
}

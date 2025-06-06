package backend

import (
	"context"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/httplog/v2"
	"github.com/go-chi/render"
	"kusionstack.io/kusion/pkg/domain/request"
	"kusionstack.io/kusion/pkg/domain/response"
	"kusionstack.io/kusion/pkg/server/handler"
	backendmanager "kusionstack.io/kusion/pkg/server/manager/backend"
	logutil "kusionstack.io/kusion/pkg/server/util/logging"
)

// @Id				createBackend
// @Summary		Create backend
// @Description	Create a new backend
// @Tags			backend
// @Accept			json
// @Produce		json
// @Param			backend	body		request.CreateBackendRequest			true	"Created backend"
// @Success		200		{object}	handler.Response{data=entity.Backend}	"Success"
// @Failure		400		{object}	error									"Bad Request"
// @Failure		401		{object}	error									"Unauthorized"
// @Failure		429		{object}	error									"Too Many Requests"
// @Failure		404		{object}	error									"Not Found"
// @Failure		500		{object}	error									"Internal Server Error"
// @Router			/api/v1/backends [post]
func (h *Handler) CreateBackend() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Getting stuff from context
		ctx := r.Context()
		logger := logutil.GetLogger(ctx)
		logger.Info("Creating backend...")

		// Decode the request body into the payload.
		var requestPayload request.CreateBackendRequest
		if err := requestPayload.Decode(r); err != nil {
			render.Render(w, r, handler.FailureResponse(ctx, err))
			return
		}

		// Validate request payload
		if err := requestPayload.Validate(); err != nil {
			render.Render(w, r, handler.FailureResponse(ctx, err))
			return
		}

		createdEntity, err := h.backendManager.CreateBackend(ctx, requestPayload)
		handler.HandleResult(w, r, ctx, err, createdEntity)
	}
}

// @Id				deleteBackend
// @Summary		Delete backend
// @Description	Delete specified backend by ID
// @Tags			backend
// @Produce		json
// @Param			backendID	path		int								true	"Backend ID"
// @Success		200			{object}	handler.Response{data=string}	"Success"
// @Failure		400			{object}	error							"Bad Request"
// @Failure		401			{object}	error							"Unauthorized"
// @Failure		429			{object}	error							"Too Many Requests"
// @Failure		404			{object}	error							"Not Found"
// @Failure		500			{object}	error							"Internal Server Error"
// @Router			/api/v1/backends/{backendID} [delete]
func (h *Handler) DeleteBackend() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Getting stuff from context
		ctx, logger, params, err := requestHelper(r)
		if err != nil {
			render.Render(w, r, handler.FailureResponse(ctx, err))
			return
		}
		logger.Info("Deleting backend...", "backendID", params.BackendID)

		err = h.backendManager.DeleteBackendByID(ctx, params.BackendID)
		handler.HandleResult(w, r, ctx, err, "Deletion Success")
	}
}

// @Id				updateBackend
// @Summary		Update backend
// @Description	Update the specified backend
// @Tags			backend
// @Accept			json
// @Produce		json
// @Param			backendID	path		int										true	"Backend ID"
// @Param			backend		body		request.UpdateBackendRequest			true	"Updated backend"
// @Success		200			{object}	handler.Response{data=entity.Backend}	"Success"
// @Failure		400			{object}	error									"Bad Request"
// @Failure		401			{object}	error									"Unauthorized"
// @Failure		429			{object}	error									"Too Many Requests"
// @Failure		404			{object}	error									"Not Found"
// @Failure		500			{object}	error									"Internal Server Error"
// @Router			/api/v1/backends/{backendID} [put]
func (h *Handler) UpdateBackend() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Getting stuff from context
		ctx, logger, params, err := requestHelper(r)
		if err != nil {
			render.Render(w, r, handler.FailureResponse(ctx, err))
			return
		}
		logger.Info("Updating backend...", "backendID", params.BackendID)

		// Decode the request body into the payload.
		var requestPayload request.UpdateBackendRequest
		if err := requestPayload.Decode(r); err != nil {
			render.Render(w, r, handler.FailureResponse(ctx, err))
			return
		}

		// Validate request payload
		if err := requestPayload.Validate(); err != nil {
			render.Render(w, r, handler.FailureResponse(ctx, err))
			return
		}

		updatedEntity, err := h.backendManager.UpdateBackendByID(ctx, params.BackendID, requestPayload)
		handler.HandleResult(w, r, ctx, err, updatedEntity)
	}
}

// @Id				getBackend
// @Summary		Get backend
// @Description	Get backend information by backend ID
// @Tags			backend
// @Produce		json
// @Param			backendID	path		int										true	"Backend ID"
// @Success		200			{object}	handler.Response{data=entity.Backend}	"Success"
// @Failure		400			{object}	error									"Bad Request"
// @Failure		401			{object}	error									"Unauthorized"
// @Failure		429			{object}	error									"Too Many Requests"
// @Failure		404			{object}	error									"Not Found"
// @Failure		500			{object}	error									"Internal Server Error"
// @Router			/api/v1/backends/{backendID} [get]
func (h *Handler) GetBackend() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Getting stuff from context
		ctx, logger, params, err := requestHelper(r)
		if err != nil {
			render.Render(w, r, handler.FailureResponse(ctx, err))
			return
		}
		logger.Info("Getting backend...", "backendID", params.BackendID)

		existingEntity, err := h.backendManager.GetBackendByID(ctx, params.BackendID)
		handler.HandleResult(w, r, ctx, err, existingEntity)
	}
}

// @Id				listBackend
// @Summary		List backends
// @Description	List all backends
// @Tags			backend
// @Produce		json
// @Param			page		query		uint														false	"The current page to fetch. Default to 1"
// @Param			pageSize	query		uint														false	"The size of the page. Default to 10"
// @Param			sortBy		query		string														false	"Which field to sort the list by. Default to id"
// @Param			ascending	query		bool														false	"Whether to sort the list in ascending order. Default to false"
// @Success		200			{object}	handler.Response{data=response.PaginatedBackendResponse}	"Success"
// @Failure		400			{object}	error														"Bad Request"
// @Failure		401			{object}	error														"Unauthorized"
// @Failure		429			{object}	error														"Too Many Requests"
// @Failure		404			{object}	error														"Not Found"
// @Failure		500			{object}	error														"Internal Server Error"
// @Router			/api/v1/backends [get]
func (h *Handler) ListBackends() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Getting stuff from context
		ctx := r.Context()
		logger := logutil.GetLogger(ctx)
		logger.Info("Listing backend...")

		query := r.URL.Query()
		filter, backendSortOptions, err := h.backendManager.BuildBackendFilterAndSortOptions(ctx, &query)
		if err != nil {
			render.Render(w, r, handler.FailureResponse(ctx, err))
			return
		}

		// List paginated backends.
		backendEntities, err := h.backendManager.ListBackends(ctx, filter, backendSortOptions)
		if err != nil {
			render.Render(w, r, handler.FailureResponse(ctx, err))
			return
		}

		paginatedResponse := response.PaginatedBackendResponse{
			Backends:    backendEntities.Backends,
			Total:       backendEntities.Total,
			CurrentPage: filter.Pagination.Page,
			PageSize:    filter.Pagination.PageSize,
		}
		handler.HandleResult(w, r, ctx, err, paginatedResponse)
	}
}

func requestHelper(r *http.Request) (context.Context, *httplog.Logger, *BackendRequestParams, error) {
	ctx := r.Context()
	backendID := chi.URLParam(r, "backendID")
	// Get stack with repository
	id, err := strconv.Atoi(backendID)
	if err != nil {
		return nil, nil, nil, backendmanager.ErrInvalidBackendID
	}
	logger := logutil.GetLogger(ctx)
	params := BackendRequestParams{
		BackendID: uint(id),
	}
	return ctx, logger, &params, nil
}

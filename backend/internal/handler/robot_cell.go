package handler

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"robot-cell-safety-envelope-validator/backend/internal/dto"
	"robot-cell-safety-envelope-validator/backend/internal/service"
)

type RobotCellHandler struct{ service *service.RobotCellService }

func NewRobotCellHandler(service *service.RobotCellService) *RobotCellHandler {
	return &RobotCellHandler{service: service}
}

func (handler *RobotCellHandler) List(context *gin.Context) {
	page, pageSize := Pagination(context)
	items, meta, err := handler.service.List(context.Request.Context(), page, pageSize, context.Query("state"), context.Query("owner_team"))
	if err != nil {
		WriteError(context, err)
		return
	}
	WritePage(context, items, meta)
}

func (handler *RobotCellHandler) Get(context *gin.Context) {
	id, err := PathID(context)
	if err != nil {
		WriteError(context, err)
		return
	}
	item, err := handler.service.Get(context.Request.Context(), id)
	if err != nil {
		WriteError(context, err)
		return
	}
	WriteData(context, http.StatusOK, item)
}

func (handler *RobotCellHandler) Create(context *gin.Context) {
	var request dto.CreateRobotCellRequest
	if err := BindAndValidate(context, &request); err != nil {
		WriteError(context, err)
		return
	}
	item, err := handler.service.Create(context.Request.Context(), request, Actor(context), RequestID(context))
	if err != nil {
		WriteError(context, err)
		return
	}
	WriteData(context, http.StatusCreated, item)
}

func (handler *RobotCellHandler) Update(context *gin.Context) {
	id, err := PathID(context)
	if err != nil {
		WriteError(context, err)
		return
	}
	var request dto.UpdateRobotCellRequest
	if err := BindAndValidate(context, &request); err != nil {
		WriteError(context, err)
		return
	}
	item, err := handler.service.Update(context.Request.Context(), id, request, Actor(context), RequestID(context))
	if err != nil {
		WriteError(context, err)
		return
	}
	WriteData(context, http.StatusOK, item)
}

func (handler *RobotCellHandler) Freeze(context *gin.Context) {
	handler.stateAction(context, handler.service.Freeze)
}

func (handler *RobotCellHandler) Deactivate(context *gin.Context) {
	handler.stateAction(context, handler.service.Deactivate)
}

func (handler *RobotCellHandler) stateAction(context *gin.Context, action func(context.Context, uint, dto.Actor, string) (dto.RobotCellResponse, error)) {
	id, err := PathID(context)
	if err != nil {
		WriteError(context, err)
		return
	}
	item, err := action(context.Request.Context(), id, Actor(context), RequestID(context))
	if err != nil {
		WriteError(context, err)
		return
	}
	WriteData(context, http.StatusOK, item)
}

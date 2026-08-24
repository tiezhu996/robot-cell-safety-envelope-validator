package router

import (
	"github.com/gin-gonic/gin"

	"robot-cell-safety-envelope-validator/backend/internal/constants"
	"robot-cell-safety-envelope-validator/backend/internal/handler"
	"robot-cell-safety-envelope-validator/backend/internal/middleware"
)

func registerSafetyZoneRoutes(group *gin.RouterGroup, target *handler.SafetyZoneHandler) {
	routes := group.Group("/zones")
	routes.GET("", target.List)
	routes.GET("/:id", target.Get)
	write := routes.Group("")
	write.Use(middleware.RBAC(constants.RoleSafetyEngineer, constants.RoleAdmin))
	write.POST("", target.Create)
	routes.PUT("/:id", target.Update)
	routes.POST("/:id/activate", target.Activate)
	routes.POST("/:id/deactivate", target.Deactivate)
}

package routes

import (
	"github.com/gin-gonic/gin"
	"github.com/wpcodevo/golang-gorm-postgres/controllers"
	"github.com/wpcodevo/golang-gorm-postgres/middleware"
)

type UserRouteController struct {
	userController controllers.UserController
}

func NewRouteUserController(userController controllers.UserController) UserRouteController {
	return UserRouteController{userController}
}

func (uc *UserRouteController) UserRoute(rg *gin.RouterGroup) {
	router := rg.Group("users")

	router.Use(middleware.DeserializeUser())

	router.GET("/me", uc.userController.GetMe)
	router.GET("/", middleware.AbacMiddleware("users", "list"), uc.userController.GetUsers)
	router.GET("/user", uc.userController.FindUser)
	//router.GET("/userInfo", uc.userController.FindUser)
	router.DELETE("/user", middleware.AbacMiddleware("users", "delete"), uc.userController.DeleteUser)
	router.PUT("/user", uc.userController.UpdateUser)
}

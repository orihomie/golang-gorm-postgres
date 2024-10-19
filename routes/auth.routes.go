package routes

import (
	"github.com/gin-gonic/gin"
	"github.com/ivegotanidea/golang-gorm-postgres/controllers"
	"github.com/ivegotanidea/golang-gorm-postgres/middleware"
)

type AuthRouteController struct {
	authController controllers.AuthController
}

func NewAuthRouteController(authController controllers.AuthController) AuthRouteController {
	return AuthRouteController{authController}
}

// @BasePath /api/v1/auth

func (rc *AuthRouteController) AuthRoute(rg *gin.RouterGroup) {
	router := rg.Group("auth")

	router.POST("/register", rc.authController.BotSignUpUser)
	router.POST("/login", rc.authController.BotSignInUser)
	router.GET("/refresh", rc.authController.RefreshAccessToken)
	router.GET("/logout", middleware.DeserializeUser(), rc.authController.LogoutUser)
}

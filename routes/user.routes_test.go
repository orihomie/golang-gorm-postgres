package routes

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/wpcodevo/golang-gorm-postgres/controllers"
	"github.com/wpcodevo/golang-gorm-postgres/initializers"
	"github.com/wpcodevo/golang-gorm-postgres/models"
	"github.com/wpcodevo/golang-gorm-postgres/utils"
	"gorm.io/gorm"
	"log"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type UserResponse struct {
	Status string `json:"status"`
	Data   struct {
		User models.UserResponse `json:"user"`
	} `json:"data"`
}

type FindUserResponse struct {
	Status string              `json:"status"`
	Data   models.UserResponse `json:"data"`
}

func getOwnerUser() models.User {
	return models.User{
		ID:             uuid.Max,
		Name:           "He Who Remains",
		Phone:          "77778889900",
		TelegramUserId: 6794234746,
		Password:       "h5sh3d", // Ensure this is hashed
		Avatar:         "https://akm-img-a-in.tosshub.com/indiatoday/images/story/202311/tom-hiddleston-in-a-still-from-loki-2-27480244-16x9_0.jpg",
		Verified:       true,
		HasProfile:     false,
		Tier:           "owner",
	}
}

func createOwnerUser(db *gorm.DB) {

	owner := getOwnerUser()

	if err := db.Where("tier = ?", "owner").FirstOrCreate(&owner).Error; err != nil {
		panic(err)
	}
}

// SetupUCRouter sets up the router for testing.
func SetupUCRouter(userController *controllers.UserController) *gin.Engine {
	r := gin.Default()

	userRouteController := NewRouteUserController(*userController)

	api := r.Group("/api")
	userRouteController.UserRoute(api)

	return r
}

func SetupUCController() controllers.UserController {
	var err error
	config, err := initializers.LoadConfig("../.")
	if err != nil {
		log.Fatal("🚀 Could not load environment variables", err)
	}

	initializers.ConnectDB(&config)
	initializers.InitCasbin(&config)

	userController := controllers.NewUserController(initializers.DB)
	userController.DB.Exec("CREATE EXTENSION IF NOT EXISTS \"uuid-ossp\"")

	createOwnerUser(userController.DB)

	// Migrate the schema
	if err := userController.DB.AutoMigrate(&models.User{}, &models.Profile{}); err != nil {
		panic("failed to migrate database")
	}

	return userController
}

func generateUser(random *rand.Rand, authRouter *gin.Engine, t *testing.T) models.UserResponse {
	name := utils.GenerateRandomStringWithPrefix(random, 10, "test-")
	phone := utils.GenerateRandomPhoneNumber(random, 0)
	telegramUserId := fmt.Sprintf("%d", rand.Int64())

	payload := fmt.Sprintf(`{"name": "%s", "phone": "%s", "telegramUserId": "%s"}`, name, phone, telegramUserId)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/auth/register", bytes.NewBuffer([]byte(payload)))
	req.Header.Set("Content-Type", "application/json")
	authRouter.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var userResponse UserResponse
	err := json.Unmarshal(w.Body.Bytes(), &userResponse)
	assert.NoError(t, err)

	user := userResponse.Data.User

	return user
}

func loginUserGetAccessToken(t *testing.T, password string, telegramUserId int64, authRouter *gin.Engine) (*http.Cookie, error) {
	var jsonResponse map[string]interface{}

	w := httptest.NewRecorder()
	payloadLogin := fmt.Sprintf(`{"telegramUserId": "%d", "password": "%s"}`, telegramUserId, password)
	loginReq, _ := http.NewRequest("POST", "/api/auth/login", bytes.NewBuffer([]byte(payloadLogin)))
	loginReq.Header.Set("Content-Type", "application/json")
	authRouter.ServeHTTP(w, loginReq)

	err := json.Unmarshal(w.Body.Bytes(), &jsonResponse)

	assert.NoError(t, err)
	status := jsonResponse["status"]

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, status, "success")
	assert.NotEmpty(t, jsonResponse["access_token"])

	// Extract refresh_token from cookies
	cookies := w.Result().Cookies()

	for _, cookie := range cookies {
		if cookie.Name == "access_token" {
			return cookie, err
		}
	}
	return nil, errors.New("cookie not found")
}

func TestUserRoutes(t *testing.T) {

	ac := SetupAuthController()
	uc := SetupUCController()

	authRouter := SetupACRouter(&ac)
	userRouter := SetupUCRouter(&uc)

	random := rand.New(rand.NewPCG(1, uint64(time.Now().Nanosecond())))

	t.Cleanup(func() {
		utils.CleanupTestUsers(uc.DB)
		utils.DropAllTables(uc.DB)
	})

	t.Run("GET /api/user/me: fail without access token ", func(t *testing.T) {

		w := httptest.NewRecorder()
		meReq, _ := http.NewRequest("GET", "/api/users/me", nil)
		meReq.Header.Set("Content-Type", "application/json")
		userRouter.ServeHTTP(w, meReq)

		var jsonResponse map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &jsonResponse)
		assert.Nil(t, err)

		assert.Equal(t, http.StatusUnauthorized, w.Code)

	})

	t.Run("GET /api/user/me: success with access token", func(t *testing.T) {
		name := utils.GenerateRandomStringWithPrefix(random, 10, "test-")
		phone := utils.GenerateRandomPhoneNumber(random, 0)
		telegramUserId := fmt.Sprintf("%d", rand.Int64())

		payload := fmt.Sprintf(`{"name": "%s", "phone": "%s", "telegramUserId": "%s"}`, name, phone, telegramUserId)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/auth/register", bytes.NewBuffer([]byte(payload)))
		req.Header.Set("Content-Type", "application/json")
		authRouter.ServeHTTP(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)

		var jsonResponse map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &jsonResponse)
		assert.NoError(t, err)

		data := jsonResponse["data"].(map[string]interface{})
		user := data["user"].(map[string]interface{})

		assert.Equal(t, "success", jsonResponse["status"])
		// Check name and phone
		assert.Equal(t, name, user["name"])
		assert.Equal(t, phone, user["phone"])

		w = httptest.NewRecorder()
		password := user["password"].(string)
		payloadLogin := fmt.Sprintf(`{"telegramUserId": "%s", "password": "%s"}`, telegramUserId, password)
		loginReq, _ := http.NewRequest("POST", "/api/auth/login", bytes.NewBuffer([]byte(payloadLogin)))
		loginReq.Header.Set("Content-Type", "application/json")
		authRouter.ServeHTTP(w, loginReq)

		err = json.Unmarshal(w.Body.Bytes(), &jsonResponse)
		status := jsonResponse["status"]

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, status, "success")
		assert.NotEmpty(t, jsonResponse["access_token"])

		// Extract refresh_token from cookies
		cookies := w.Result().Cookies()
		var accessTokenCookie *http.Cookie
		for _, cookie := range cookies {
			if cookie.Name == "access_token" {
				accessTokenCookie = cookie
				break
			}
		}

		w = httptest.NewRecorder()
		meReq, _ := http.NewRequest("GET", "/api/users/me", nil)
		meReq.Header.Set("Content-Type", "application/json")
		meReq.AddCookie(&http.Cookie{Name: accessTokenCookie.Name, Value: accessTokenCookie.Value})
		userRouter.ServeHTTP(w, meReq)

		jsonResponse = make(map[string]interface{})
		err = json.Unmarshal(w.Body.Bytes(), &jsonResponse)
		assert.Nil(t, err)

		assert.Equal(t, http.StatusOK, w.Code)

		data = jsonResponse["data"].(map[string]interface{})
		assert.NotEmpty(t, data)

	})

	t.Run("GET /api/user: no access_token, forbidden to list users", func(t *testing.T) {

		w := httptest.NewRecorder()
		meReq, _ := http.NewRequest("GET", "/api/users/", nil)
		meReq.Header.Set("Content-Type", "application/json")
		userRouter.ServeHTTP(w, meReq)

		jsonResponse := make(map[string]interface{})
		err := json.Unmarshal(w.Body.Bytes(), &jsonResponse)
		assert.Nil(t, err)

		assert.Equal(t, http.StatusUnauthorized, w.Code)

	})

	t.Run("GET /api/user: basic user, forbidden to list users", func(t *testing.T) {
		name := utils.GenerateRandomStringWithPrefix(random, 10, "test-")
		phone := utils.GenerateRandomPhoneNumber(random, 0)
		telegramUserId := fmt.Sprintf("%d", rand.Int64())

		payload := fmt.Sprintf(`{"name": "%s", "phone": "%s", "telegramUserId": "%s"}`, name, phone, telegramUserId)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/auth/register", bytes.NewBuffer([]byte(payload)))
		req.Header.Set("Content-Type", "application/json")
		authRouter.ServeHTTP(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)

		var jsonResponse map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &jsonResponse)
		assert.NoError(t, err)

		data := jsonResponse["data"].(map[string]interface{})
		user := data["user"].(map[string]interface{})

		assert.Equal(t, "success", jsonResponse["status"])
		// Check name and phone
		assert.Equal(t, name, user["name"])
		assert.Equal(t, phone, user["phone"])

		w = httptest.NewRecorder()
		password := user["password"].(string)
		payloadLogin := fmt.Sprintf(`{"telegramUserId": "%s", "password": "%s"}`, telegramUserId, password)
		loginReq, _ := http.NewRequest("POST", "/api/auth/login", bytes.NewBuffer([]byte(payloadLogin)))
		loginReq.Header.Set("Content-Type", "application/json")
		authRouter.ServeHTTP(w, loginReq)

		err = json.Unmarshal(w.Body.Bytes(), &jsonResponse)
		status := jsonResponse["status"]

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, status, "success")
		assert.NotEmpty(t, jsonResponse["access_token"])

		// Extract refresh_token from cookies
		cookies := w.Result().Cookies()
		var accessTokenCookie *http.Cookie
		for _, cookie := range cookies {
			if cookie.Name == "access_token" {
				accessTokenCookie = cookie
				break
			}
		}

		w = httptest.NewRecorder()
		meReq, _ := http.NewRequest("GET", "/api/users/", nil)
		meReq.Header.Set("Content-Type", "application/json")
		meReq.AddCookie(&http.Cookie{Name: accessTokenCookie.Name, Value: accessTokenCookie.Value})
		userRouter.ServeHTTP(w, meReq)

		jsonResponse = make(map[string]interface{})
		err = json.Unmarshal(w.Body.Bytes(), &jsonResponse)
		assert.Nil(t, err)

		assert.Equal(t, http.StatusForbidden, w.Code)

	})

	t.Run("GET /api/user: moderator, success list users", func(t *testing.T) {
		name := utils.GenerateRandomStringWithPrefix(random, 10, "test-")
		phone := utils.GenerateRandomPhoneNumber(random, 0)
		telegramUserId := fmt.Sprintf("%d", rand.Int64())

		payload := fmt.Sprintf(`{"name": "%s", "phone": "%s", "telegramUserId": "%s"}`, name, phone, telegramUserId)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/auth/register", bytes.NewBuffer([]byte(payload)))
		req.Header.Set("Content-Type", "application/json")
		authRouter.ServeHTTP(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)

		var jsonResponse map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &jsonResponse)
		assert.NoError(t, err)

		data := jsonResponse["data"].(map[string]interface{})
		user := data["user"].(map[string]interface{})

		assert.Equal(t, "success", jsonResponse["status"])
		// Check name and phone
		assert.Equal(t, name, user["name"])
		assert.Equal(t, phone, user["phone"])

		tx := initializers.DB.Model(&models.User{}).Where("id = ?", user["id"]).Update("tier", "moderator")

		assert.NoError(t, tx.Error)
		assert.Equal(t, int64(1), tx.RowsAffected)

		w = httptest.NewRecorder()
		password := user["password"].(string)
		payloadLogin := fmt.Sprintf(`{"telegramUserId": "%s", "password": "%s"}`, telegramUserId, password)
		loginReq, _ := http.NewRequest("POST", "/api/auth/login", bytes.NewBuffer([]byte(payloadLogin)))
		loginReq.Header.Set("Content-Type", "application/json")
		authRouter.ServeHTTP(w, loginReq)

		jsonResponse = make(map[string]interface{})
		err = json.Unmarshal(w.Body.Bytes(), &jsonResponse)
		status := jsonResponse["status"]

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, status, "success")
		assert.NotEmpty(t, jsonResponse["access_token"])

		// Extract refresh_token from cookies
		cookies := w.Result().Cookies()
		var accessTokenCookie *http.Cookie
		for _, cookie := range cookies {
			if cookie.Name == "access_token" {
				accessTokenCookie = cookie
				break
			}
		}

		w = httptest.NewRecorder()
		meReq, _ := http.NewRequest("GET", "/api/users/", nil)
		meReq.Header.Set("Content-Type", "application/json")
		meReq.AddCookie(&http.Cookie{Name: accessTokenCookie.Name, Value: accessTokenCookie.Value})
		userRouter.ServeHTTP(w, meReq)

		jsonResponse = make(map[string]interface{})
		err = json.Unmarshal(w.Body.Bytes(), &jsonResponse)
		assert.Nil(t, err)

		assert.Equal(t, http.StatusOK, w.Code)

	})

	t.Run("GET /api/user: admin, success list users", func(t *testing.T) {
		name := utils.GenerateRandomStringWithPrefix(random, 10, "test-")
		phone := utils.GenerateRandomPhoneNumber(random, 0)
		telegramUserId := fmt.Sprintf("%d", rand.Int64())

		payload := fmt.Sprintf(`{"name": "%s", "phone": "%s", "telegramUserId": "%s"}`, name, phone, telegramUserId)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/auth/register", bytes.NewBuffer([]byte(payload)))
		req.Header.Set("Content-Type", "application/json")
		authRouter.ServeHTTP(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)

		var jsonResponse map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &jsonResponse)
		assert.NoError(t, err)

		data := jsonResponse["data"].(map[string]interface{})
		user := data["user"].(map[string]interface{})

		assert.Equal(t, "success", jsonResponse["status"])
		// Check name and phone
		assert.Equal(t, name, user["name"])
		assert.Equal(t, phone, user["phone"])

		tx := initializers.DB.Model(&models.User{}).Where("id = ?", user["id"]).Update("tier", "admin")

		assert.NoError(t, tx.Error)
		assert.Equal(t, int64(1), tx.RowsAffected)

		w = httptest.NewRecorder()
		password := user["password"].(string)
		payloadLogin := fmt.Sprintf(`{"telegramUserId": "%s", "password": "%s"}`, telegramUserId, password)
		loginReq, _ := http.NewRequest("POST", "/api/auth/login", bytes.NewBuffer([]byte(payloadLogin)))
		loginReq.Header.Set("Content-Type", "application/json")
		authRouter.ServeHTTP(w, loginReq)

		jsonResponse = make(map[string]interface{})
		err = json.Unmarshal(w.Body.Bytes(), &jsonResponse)
		status := jsonResponse["status"]

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, status, "success")
		assert.NotEmpty(t, jsonResponse["access_token"])

		// Extract refresh_token from cookies
		cookies := w.Result().Cookies()
		var accessTokenCookie *http.Cookie
		for _, cookie := range cookies {
			if cookie.Name == "access_token" {
				accessTokenCookie = cookie
				break
			}
		}

		w = httptest.NewRecorder()
		meReq, _ := http.NewRequest("GET", "/api/users/", nil)
		meReq.Header.Set("Content-Type", "application/json")
		meReq.AddCookie(&http.Cookie{Name: accessTokenCookie.Name, Value: accessTokenCookie.Value})
		userRouter.ServeHTTP(w, meReq)

		jsonResponse = make(map[string]interface{})
		err = json.Unmarshal(w.Body.Bytes(), &jsonResponse)
		assert.Nil(t, err)

		assert.Equal(t, http.StatusOK, w.Code)

	})

	t.Run("GET /api/users/user: fail without access token", func(t *testing.T) {
		name := utils.GenerateRandomStringWithPrefix(random, 10, "test-")
		phone := utils.GenerateRandomPhoneNumber(random, 0)
		telegramUserId := fmt.Sprintf("%d", rand.Int64())

		payload := fmt.Sprintf(`{"name": "%s", "phone": "%s", "telegramUserId": "%s"}`, name, phone, telegramUserId)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/auth/register", bytes.NewBuffer([]byte(payload)))
		req.Header.Set("Content-Type", "application/json")
		authRouter.ServeHTTP(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)

		var jsonResponse map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &jsonResponse)
		assert.NoError(t, err)

		data := jsonResponse["data"].(map[string]interface{})
		user := data["user"].(map[string]interface{})

		assert.NotEmpty(t, data)
		assert.NotEmpty(t, user)

		w = httptest.NewRecorder()

		jsonResponse = make(map[string]interface{})
		url := fmt.Sprintf("/api/users/user?phone=%s", user["phone"])
		findUserReq, _ := http.NewRequest("GET", url, nil)
		findUserReq.Header.Set("Content-Type", "application/json")
		userRouter.ServeHTTP(w, findUserReq)

		err = json.Unmarshal(w.Body.Bytes(), &jsonResponse)
		assert.Nil(t, err)

		assert.Equal(t, http.StatusUnauthorized, w.Code)

	})

	t.Run("GET /api/users/user: success by phone with access token", func(t *testing.T) {
		firstUser := generateUser(random, authRouter, t)
		secondUser := generateUser(random, authRouter, t)

		var jsonResponse map[string]interface{}

		w := httptest.NewRecorder()
		password := firstUser.Password
		telegramUserId := firstUser.TelegramUserID
		payloadLogin := fmt.Sprintf(`{"telegramUserId": "%d", "password": "%s"}`, telegramUserId, password)
		loginReq, _ := http.NewRequest("POST", "/api/auth/login", bytes.NewBuffer([]byte(payloadLogin)))
		loginReq.Header.Set("Content-Type", "application/json")
		authRouter.ServeHTTP(w, loginReq)

		err := json.Unmarshal(w.Body.Bytes(), &jsonResponse)

		assert.NoError(t, err)
		status := jsonResponse["status"]

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, status, "success")
		assert.NotEmpty(t, jsonResponse["access_token"])

		// Extract refresh_token from cookies
		cookies := w.Result().Cookies()
		var accessTokenCookie *http.Cookie
		for _, cookie := range cookies {
			if cookie.Name == "access_token" {
				accessTokenCookie = cookie
				break
			}
		}

		w = httptest.NewRecorder()

		url := fmt.Sprintf("/api/users/user?phone=%s", secondUser.Phone)
		findUserReq, _ := http.NewRequest("GET", url, nil)
		findUserReq.Header.Set("Content-Type", "application/json")
		findUserReq.AddCookie(&http.Cookie{Name: accessTokenCookie.Name, Value: accessTokenCookie.Value})
		userRouter.ServeHTTP(w, findUserReq)

		var userResponse FindUserResponse
		err = json.Unmarshal(w.Body.Bytes(), &userResponse)
		assert.Nil(t, err)
		assert.NotEmpty(t, userResponse)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("GET /api/users/user: success by id with access token", func(t *testing.T) {
		firstUser := generateUser(random, authRouter, t)
		secondUser := generateUser(random, authRouter, t)

		var jsonResponse map[string]interface{}

		w := httptest.NewRecorder()
		password := firstUser.Password
		telegramUserId := firstUser.TelegramUserID
		payloadLogin := fmt.Sprintf(`{"telegramUserId": "%d", "password": "%s"}`, telegramUserId, password)
		loginReq, _ := http.NewRequest("POST", "/api/auth/login", bytes.NewBuffer([]byte(payloadLogin)))
		loginReq.Header.Set("Content-Type", "application/json")
		authRouter.ServeHTTP(w, loginReq)

		err := json.Unmarshal(w.Body.Bytes(), &jsonResponse)

		assert.NoError(t, err)
		status := jsonResponse["status"]

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, status, "success")
		assert.NotEmpty(t, jsonResponse["access_token"])

		// Extract refresh_token from cookies
		cookies := w.Result().Cookies()
		var accessTokenCookie *http.Cookie
		for _, cookie := range cookies {
			if cookie.Name == "access_token" {
				accessTokenCookie = cookie
				break
			}
		}

		w = httptest.NewRecorder()

		url := fmt.Sprintf("/api/users/user?id=%s", secondUser.ID)
		findUserReq, _ := http.NewRequest("GET", url, nil)
		findUserReq.Header.Set("Content-Type", "application/json")
		findUserReq.AddCookie(&http.Cookie{Name: accessTokenCookie.Name, Value: accessTokenCookie.Value})
		userRouter.ServeHTTP(w, findUserReq)

		var userResponse FindUserResponse
		err = json.Unmarshal(w.Body.Bytes(), &userResponse)
		assert.Nil(t, err)
		assert.NotEmpty(t, userResponse)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("GET /api/users/user: success by telegramId with access token", func(t *testing.T) {
		firstUser := generateUser(random, authRouter, t)
		secondUser := generateUser(random, authRouter, t)

		var jsonResponse map[string]interface{}

		w := httptest.NewRecorder()
		password := firstUser.Password
		telegramUserId := firstUser.TelegramUserID
		payloadLogin := fmt.Sprintf(`{"telegramUserId": "%d", "password": "%s"}`, telegramUserId, password)
		loginReq, _ := http.NewRequest("POST", "/api/auth/login", bytes.NewBuffer([]byte(payloadLogin)))
		loginReq.Header.Set("Content-Type", "application/json")
		authRouter.ServeHTTP(w, loginReq)

		err := json.Unmarshal(w.Body.Bytes(), &jsonResponse)

		assert.NoError(t, err)
		status := jsonResponse["status"]

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, status, "success")
		assert.NotEmpty(t, jsonResponse["access_token"])

		// Extract refresh_token from cookies
		cookies := w.Result().Cookies()
		var accessTokenCookie *http.Cookie
		for _, cookie := range cookies {
			if cookie.Name == "access_token" {
				accessTokenCookie = cookie
				break
			}
		}

		w = httptest.NewRecorder()

		url := fmt.Sprintf("/api/users/user?telegramUserId=%d", secondUser.TelegramUserID)
		findUserReq, _ := http.NewRequest("GET", url, nil)
		findUserReq.Header.Set("Content-Type", "application/json")
		findUserReq.AddCookie(&http.Cookie{Name: accessTokenCookie.Name, Value: accessTokenCookie.Value})
		userRouter.ServeHTTP(w, findUserReq)

		var userResponse FindUserResponse
		err = json.Unmarshal(w.Body.Bytes(), &userResponse)
		assert.Nil(t, err)
		assert.NotEmpty(t, userResponse)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("GET /api/users/user: 404 non existing phone with access token", func(t *testing.T) {
		firstUser := generateUser(random, authRouter, t)

		var jsonResponse map[string]interface{}

		w := httptest.NewRecorder()
		password := firstUser.Password
		telegramUserId := firstUser.TelegramUserID
		payloadLogin := fmt.Sprintf(`{"telegramUserId": "%d", "password": "%s"}`, telegramUserId, password)
		loginReq, _ := http.NewRequest("POST", "/api/auth/login", bytes.NewBuffer([]byte(payloadLogin)))
		loginReq.Header.Set("Content-Type", "application/json")
		authRouter.ServeHTTP(w, loginReq)

		err := json.Unmarshal(w.Body.Bytes(), &jsonResponse)

		assert.NoError(t, err)
		status := jsonResponse["status"]

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, status, "success")
		assert.NotEmpty(t, jsonResponse["access_token"])

		// Extract refresh_token from cookies
		cookies := w.Result().Cookies()
		var accessTokenCookie *http.Cookie
		for _, cookie := range cookies {
			if cookie.Name == "access_token" {
				accessTokenCookie = cookie
				break
			}
		}

		w = httptest.NewRecorder()

		url := fmt.Sprintf("/api/users/user?phone=%s", "7000000000")
		findUserReq, _ := http.NewRequest("GET", url, nil)
		findUserReq.Header.Set("Content-Type", "application/json")
		findUserReq.AddCookie(&http.Cookie{Name: accessTokenCookie.Name, Value: accessTokenCookie.Value})
		userRouter.ServeHTTP(w, findUserReq)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("GET /api/users/user: 404 non existing id with access token", func(t *testing.T) {
		firstUser := generateUser(random, authRouter, t)

		var jsonResponse map[string]interface{}

		w := httptest.NewRecorder()
		password := firstUser.Password
		telegramUserId := firstUser.TelegramUserID
		payloadLogin := fmt.Sprintf(`{"telegramUserId": "%d", "password": "%s"}`, telegramUserId, password)
		loginReq, _ := http.NewRequest("POST", "/api/auth/login", bytes.NewBuffer([]byte(payloadLogin)))
		loginReq.Header.Set("Content-Type", "application/json")
		authRouter.ServeHTTP(w, loginReq)

		err := json.Unmarshal(w.Body.Bytes(), &jsonResponse)

		assert.NoError(t, err)
		status := jsonResponse["status"]

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, status, "success")
		assert.NotEmpty(t, jsonResponse["access_token"])

		// Extract refresh_token from cookies
		cookies := w.Result().Cookies()
		var accessTokenCookie *http.Cookie
		for _, cookie := range cookies {
			if cookie.Name == "access_token" {
				accessTokenCookie = cookie
				break
			}
		}

		w = httptest.NewRecorder()

		url := fmt.Sprintf("/api/users/user?id=%s", "00000000-0000-0000-0000-000000000000")
		findUserReq, _ := http.NewRequest("GET", url, nil)
		findUserReq.Header.Set("Content-Type", "application/json")
		findUserReq.AddCookie(&http.Cookie{Name: accessTokenCookie.Name, Value: accessTokenCookie.Value})
		userRouter.ServeHTTP(w, findUserReq)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("GET /api/users/user: 404 non existing telegramId with access token", func(t *testing.T) {
		firstUser := generateUser(random, authRouter, t)

		var jsonResponse map[string]interface{}

		w := httptest.NewRecorder()
		password := firstUser.Password
		telegramUserId := firstUser.TelegramUserID
		payloadLogin := fmt.Sprintf(`{"telegramUserId": "%d", "password": "%s"}`, telegramUserId, password)
		loginReq, _ := http.NewRequest("POST", "/api/auth/login", bytes.NewBuffer([]byte(payloadLogin)))
		loginReq.Header.Set("Content-Type", "application/json")
		authRouter.ServeHTTP(w, loginReq)

		err := json.Unmarshal(w.Body.Bytes(), &jsonResponse)

		assert.NoError(t, err)
		status := jsonResponse["status"]

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, status, "success")
		assert.NotEmpty(t, jsonResponse["access_token"])

		// Extract refresh_token from cookies
		cookies := w.Result().Cookies()
		var accessTokenCookie *http.Cookie
		for _, cookie := range cookies {
			if cookie.Name == "access_token" {
				accessTokenCookie = cookie
				break
			}
		}

		w = httptest.NewRecorder()

		url := fmt.Sprintf("/api/users/user?telegramUserId=%d", 1991)
		findUserReq, _ := http.NewRequest("GET", url, nil)
		findUserReq.Header.Set("Content-Type", "application/json")
		findUserReq.AddCookie(&http.Cookie{Name: accessTokenCookie.Name, Value: accessTokenCookie.Value})
		userRouter.ServeHTTP(w, findUserReq)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("DELETE /api/users/user: success with access token", func(t *testing.T) {
		firstUser := generateUser(random, authRouter, t)

		var jsonResponse map[string]interface{}

		w := httptest.NewRecorder()
		password := firstUser.Password
		telegramUserId := firstUser.TelegramUserID
		payloadLogin := fmt.Sprintf(`{"telegramUserId": "%d", "password": "%s"}`, telegramUserId, password)
		loginReq, _ := http.NewRequest("POST", "/api/auth/login", bytes.NewBuffer([]byte(payloadLogin)))
		loginReq.Header.Set("Content-Type", "application/json")
		authRouter.ServeHTTP(w, loginReq)

		err := json.Unmarshal(w.Body.Bytes(), &jsonResponse)

		assert.NoError(t, err)
		status := jsonResponse["status"]

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, status, "success")
		assert.NotEmpty(t, jsonResponse["access_token"])

		// Extract refresh_token from cookies
		cookies := w.Result().Cookies()
		var accessTokenCookie *http.Cookie
		for _, cookie := range cookies {
			if cookie.Name == "access_token" {
				accessTokenCookie = cookie
				break
			}
		}

		w = httptest.NewRecorder()

		url := fmt.Sprintf("/api/users/user")
		findUserReq, _ := http.NewRequest(http.MethodDelete, url, nil)
		findUserReq.Header.Set("Content-Type", "application/json")
		findUserReq.AddCookie(&http.Cookie{Name: accessTokenCookie.Name, Value: accessTokenCookie.Value})
		userRouter.ServeHTTP(w, findUserReq)

		assert.Equal(t, http.StatusNoContent, w.Code)

		jsonResponse = make(map[string]interface{})

		w = httptest.NewRecorder()
		payloadLogin = fmt.Sprintf(`{"telegramUserId": "%d", "password": "%s"}`, telegramUserId, password)
		loginReq2, _ := http.NewRequest("POST", "/api/auth/login", bytes.NewBuffer([]byte(payloadLogin)))
		loginReq2.Header.Set("Content-Type", "application/json")
		authRouter.ServeHTTP(w, loginReq2)

		err = json.Unmarshal(w.Body.Bytes(), &jsonResponse)

		assert.NoError(t, err)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Equal(t, jsonResponse["status"], "fail")
		assert.Nil(t, jsonResponse["access_token"])
	})

	t.Run("DELETE /api/users/user: fail without access token", func(t *testing.T) {
		var jsonResponse map[string]interface{}

		w := httptest.NewRecorder()

		url := fmt.Sprintf("/api/users/user")
		delUserReq, _ := http.NewRequest(http.MethodDelete, url, nil)
		delUserReq.Header.Set("Content-Type", "application/json")
		userRouter.ServeHTTP(w, delUserReq)

		err := json.Unmarshal(w.Body.Bytes(), &jsonResponse)

		assert.NoError(t, err)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
		assert.Equal(t, jsonResponse["message"], "You are not logged in")
		assert.Equal(t, jsonResponse["status"], "fail")
	})

	t.Run("DELETE /api/users/user: success moderator deletes user", func(t *testing.T) {
		firstUser := generateUser(random, authRouter, t)
		secondUser := generateUser(random, authRouter, t)

		tx := initializers.DB.Model(&models.User{}).Where("id = ?", firstUser.ID).Update("tier", "moderator")
		assert.NoError(t, tx.Error)
		assert.Equal(t, int64(1), tx.RowsAffected)

		var jsonResponse map[string]interface{}

		w := httptest.NewRecorder()
		password := firstUser.Password
		telegramUserId := firstUser.TelegramUserID
		payloadLogin := fmt.Sprintf(`{"telegramUserId": "%d", "password": "%s"}`, telegramUserId, password)
		loginReq, _ := http.NewRequest("POST", "/api/auth/login", bytes.NewBuffer([]byte(payloadLogin)))
		loginReq.Header.Set("Content-Type", "application/json")
		authRouter.ServeHTTP(w, loginReq)

		err := json.Unmarshal(w.Body.Bytes(), &jsonResponse)

		assert.NoError(t, err)
		status := jsonResponse["status"]

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, status, "success")
		assert.NotEmpty(t, jsonResponse["access_token"])

		// Extract refresh_token from cookies
		cookies := w.Result().Cookies()
		var accessTokenCookie *http.Cookie
		for _, cookie := range cookies {
			if cookie.Name == "access_token" {
				accessTokenCookie = cookie
				break
			}
		}

		w = httptest.NewRecorder()

		url := fmt.Sprintf("/api/users/user/%s", secondUser.ID)
		delUserReq, _ := http.NewRequest(http.MethodDelete, url, nil)
		delUserReq.Header.Set("Content-Type", "application/json")
		delUserReq.AddCookie(&http.Cookie{Name: accessTokenCookie.Name, Value: accessTokenCookie.Value})
		userRouter.ServeHTTP(w, delUserReq)

		assert.Empty(t, w.Body.String())

		assert.Equal(t, http.StatusNoContent, w.Code)

		w = httptest.NewRecorder()

		findUserUrl := fmt.Sprintf("/api/users/user?id=%s", secondUser.ID)
		findUserReq, _ := http.NewRequest("GET", findUserUrl, nil)
		findUserReq.Header.Set("Content-Type", "application/json")
		findUserReq.AddCookie(&http.Cookie{Name: accessTokenCookie.Name, Value: accessTokenCookie.Value})
		userRouter.ServeHTTP(w, findUserReq)

		var userResponse FindUserResponse
		err = json.Unmarshal(w.Body.Bytes(), &userResponse)
		assert.Nil(t, err)
		assert.Equal(t, userResponse.Status, "fail")

		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("DELETE /api/users/user: success admin deletes user", func(t *testing.T) {
		firstUser := generateUser(random, authRouter, t)
		secondUser := generateUser(random, authRouter, t)

		tx := initializers.DB.Model(&models.User{}).Where("id = ?", firstUser.ID).Update("tier", "admin")
		assert.NoError(t, tx.Error)
		assert.Equal(t, int64(1), tx.RowsAffected)

		var jsonResponse map[string]interface{}

		w := httptest.NewRecorder()
		password := firstUser.Password
		telegramUserId := firstUser.TelegramUserID
		payloadLogin := fmt.Sprintf(`{"telegramUserId": "%d", "password": "%s"}`, telegramUserId, password)
		loginReq, _ := http.NewRequest("POST", "/api/auth/login", bytes.NewBuffer([]byte(payloadLogin)))
		loginReq.Header.Set("Content-Type", "application/json")
		authRouter.ServeHTTP(w, loginReq)

		err := json.Unmarshal(w.Body.Bytes(), &jsonResponse)

		assert.NoError(t, err)
		status := jsonResponse["status"]

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, status, "success")
		assert.NotEmpty(t, jsonResponse["access_token"])

		// Extract refresh_token from cookies
		cookies := w.Result().Cookies()
		var accessTokenCookie *http.Cookie
		for _, cookie := range cookies {
			if cookie.Name == "access_token" {
				accessTokenCookie = cookie
				break
			}
		}

		w = httptest.NewRecorder()

		url := fmt.Sprintf("/api/users/user/%s", secondUser.ID)
		delUserReq, _ := http.NewRequest(http.MethodDelete, url, nil)
		delUserReq.Header.Set("Content-Type", "application/json")
		delUserReq.AddCookie(&http.Cookie{Name: accessTokenCookie.Name, Value: accessTokenCookie.Value})
		userRouter.ServeHTTP(w, delUserReq)

		assert.Empty(t, w.Body.String())

		assert.Equal(t, http.StatusNoContent, w.Code)

		w = httptest.NewRecorder()

		findUserUrl := fmt.Sprintf("/api/users/user?id=%s", secondUser.ID)
		findUserReq, _ := http.NewRequest("GET", findUserUrl, nil)
		findUserReq.Header.Set("Content-Type", "application/json")
		findUserReq.AddCookie(&http.Cookie{Name: accessTokenCookie.Name, Value: accessTokenCookie.Value})
		userRouter.ServeHTTP(w, findUserReq)

		var userResponse FindUserResponse
		err = json.Unmarshal(w.Body.Bytes(), &userResponse)
		assert.Nil(t, err)
		assert.Equal(t, userResponse.Status, "fail")

		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("DELETE /api/users/user: fail moderator deletes moderator", func(t *testing.T) {
		firstUser := generateUser(random, authRouter, t)
		secondUser := generateUser(random, authRouter, t)

		tx := initializers.DB.Model(&models.User{}).Where("id = ?", firstUser.ID).Update("tier", "moderator")
		assert.NoError(t, tx.Error)
		assert.Equal(t, int64(1), tx.RowsAffected)

		tx = initializers.DB.Model(&models.User{}).Where("id = ?", secondUser.ID).Update("tier", "moderator")
		assert.NoError(t, tx.Error)
		assert.Equal(t, int64(1), tx.RowsAffected)

		accessTokenCookie, err := loginUserGetAccessToken(t, firstUser.Password, firstUser.TelegramUserID, authRouter)

		if err != nil {
			panic(err)
		}

		w := httptest.NewRecorder()

		url := fmt.Sprintf("/api/users/user/%s", secondUser.ID)
		delUserReq, _ := http.NewRequest(http.MethodDelete, url, nil)
		delUserReq.Header.Set("Content-Type", "application/json")
		delUserReq.AddCookie(&http.Cookie{Name: accessTokenCookie.Name, Value: accessTokenCookie.Value})
		userRouter.ServeHTTP(w, delUserReq)

		var jsonResponse map[string]interface{}

		err = json.Unmarshal(w.Body.Bytes(), &jsonResponse)
		assert.Equal(t, jsonResponse["status"], "fail")

		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("DELETE /api/users/user: fail moderator deletes admin", func(t *testing.T) {
		firstUser := generateUser(random, authRouter, t)
		secondUser := generateUser(random, authRouter, t)

		tx := initializers.DB.Model(&models.User{}).Where("id = ?", firstUser.ID).Update("tier", "moderator")
		assert.NoError(t, tx.Error)
		assert.Equal(t, int64(1), tx.RowsAffected)

		tx = initializers.DB.Model(&models.User{}).Where("id = ?", secondUser.ID).Update("tier", "admin")
		assert.NoError(t, tx.Error)
		assert.Equal(t, int64(1), tx.RowsAffected)

		accessTokenCookie, err := loginUserGetAccessToken(t, firstUser.Password, firstUser.TelegramUserID, authRouter)

		if err != nil {
			panic(err)
		}

		w := httptest.NewRecorder()

		url := fmt.Sprintf("/api/users/user/%s", secondUser.ID)
		delUserReq, _ := http.NewRequest(http.MethodDelete, url, nil)
		delUserReq.Header.Set("Content-Type", "application/json")
		delUserReq.AddCookie(&http.Cookie{Name: accessTokenCookie.Name, Value: accessTokenCookie.Value})
		userRouter.ServeHTTP(w, delUserReq)

		var jsonResponse map[string]interface{}

		err = json.Unmarshal(w.Body.Bytes(), &jsonResponse)
		assert.Equal(t, jsonResponse["status"], "fail")

		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("DELETE /api/users/user: success admin deletes moderator", func(t *testing.T) {
		firstUser := generateUser(random, authRouter, t)
		secondUser := generateUser(random, authRouter, t)

		tx := initializers.DB.Model(&models.User{}).Where("id = ?", firstUser.ID).Update("tier", "admin")
		assert.NoError(t, tx.Error)
		assert.Equal(t, int64(1), tx.RowsAffected)

		tx = initializers.DB.Model(&models.User{}).Where("id = ?", secondUser.ID).Update("tier", "moderator")
		assert.NoError(t, tx.Error)
		assert.Equal(t, int64(1), tx.RowsAffected)

		accessTokenCookie, err := loginUserGetAccessToken(t, firstUser.Password, firstUser.TelegramUserID, authRouter)

		if err != nil {
			panic(err)
		}

		w := httptest.NewRecorder()

		url := fmt.Sprintf("/api/users/user/%s", secondUser.ID)
		delUserReq, _ := http.NewRequest(http.MethodDelete, url, nil)
		delUserReq.Header.Set("Content-Type", "application/json")
		delUserReq.AddCookie(&http.Cookie{Name: accessTokenCookie.Name, Value: accessTokenCookie.Value})
		userRouter.ServeHTTP(w, delUserReq)

		var jsonResponse map[string]interface{}

		err = json.Unmarshal(w.Body.Bytes(), &jsonResponse)
		assert.Nil(t, jsonResponse)

		assert.Equal(t, http.StatusNoContent, w.Code)
	})

	t.Run("DELETE /api/users/user: fail admin deletes admin", func(t *testing.T) {
		firstUser := generateUser(random, authRouter, t)
		secondUser := generateUser(random, authRouter, t)

		tx := initializers.DB.Model(&models.User{}).Where("id = ?", firstUser.ID).Update("tier", "admin")
		assert.NoError(t, tx.Error)
		assert.Equal(t, int64(1), tx.RowsAffected)

		tx = initializers.DB.Model(&models.User{}).Where("id = ?", secondUser.ID).Update("tier", "admin")
		assert.NoError(t, tx.Error)
		assert.Equal(t, int64(1), tx.RowsAffected)

		accessTokenCookie, err := loginUserGetAccessToken(t, firstUser.Password, firstUser.TelegramUserID, authRouter)

		if err != nil {
			panic(err)
		}

		w := httptest.NewRecorder()

		url := fmt.Sprintf("/api/users/user/%s", secondUser.ID)
		delUserReq, _ := http.NewRequest(http.MethodDelete, url, nil)
		delUserReq.Header.Set("Content-Type", "application/json")
		delUserReq.AddCookie(&http.Cookie{Name: accessTokenCookie.Name, Value: accessTokenCookie.Value})
		userRouter.ServeHTTP(w, delUserReq)

		var jsonResponse map[string]interface{}

		err = json.Unmarshal(w.Body.Bytes(), &jsonResponse)
		assert.Equal(t, jsonResponse["status"], "fail")

		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("DELETE /api/users/user: success owner deletes admin", func(t *testing.T) {
		owner := getOwnerUser()
		secondUser := generateUser(random, authRouter, t)

		tx := initializers.DB.Model(&models.User{}).Where("id = ?", secondUser.ID).Update("tier", "admin")
		assert.NoError(t, tx.Error)
		assert.Equal(t, int64(1), tx.RowsAffected)

		accessTokenCookie, err := loginUserGetAccessToken(t, owner.Password, owner.TelegramUserId, authRouter)

		if err != nil {
			panic(err)
		}

		w := httptest.NewRecorder()

		url := fmt.Sprintf("/api/users/user/%s", secondUser.ID)
		delUserReq, _ := http.NewRequest(http.MethodDelete, url, nil)
		delUserReq.Header.Set("Content-Type", "application/json")
		delUserReq.AddCookie(&http.Cookie{Name: accessTokenCookie.Name, Value: accessTokenCookie.Value})
		userRouter.ServeHTTP(w, delUserReq)

		var jsonResponse map[string]interface{}

		err = json.Unmarshal(w.Body.Bytes(), &jsonResponse)
		assert.Nil(t, jsonResponse)

		assert.Equal(t, http.StatusNoContent, w.Code)
	})

	t.Run("DELETE /api/users/user: fail moderator deletes owner", func(t *testing.T) {
		firstUser := generateUser(random, authRouter, t)
		secondUser := getOwnerUser()

		tx := initializers.DB.Model(&models.User{}).Where("id = ?", firstUser.ID).Update("tier", "moderator")
		assert.NoError(t, tx.Error)
		assert.Equal(t, int64(1), tx.RowsAffected)

		accessTokenCookie, err := loginUserGetAccessToken(t, firstUser.Password, firstUser.TelegramUserID, authRouter)

		if err != nil {
			panic(err)
		}

		w := httptest.NewRecorder()

		url := fmt.Sprintf("/api/users/user/%s", secondUser.ID)
		delUserReq, _ := http.NewRequest(http.MethodDelete, url, nil)
		delUserReq.Header.Set("Content-Type", "application/json")
		delUserReq.AddCookie(&http.Cookie{Name: accessTokenCookie.Name, Value: accessTokenCookie.Value})
		userRouter.ServeHTTP(w, delUserReq)

		var jsonResponse map[string]interface{}

		err = json.Unmarshal(w.Body.Bytes(), &jsonResponse)
		assert.Equal(t, jsonResponse["status"], "fail")

		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("DELETE /api/users/user: fail admin deletes owner", func(t *testing.T) {
		firstUser := generateUser(random, authRouter, t)
		secondUser := getOwnerUser()

		tx := initializers.DB.Model(&models.User{}).Where("id = ?", firstUser.ID).Update("tier", "admin")
		assert.NoError(t, tx.Error)
		assert.Equal(t, int64(1), tx.RowsAffected)

		accessTokenCookie, err := loginUserGetAccessToken(t, firstUser.Password, firstUser.TelegramUserID, authRouter)

		if err != nil {
			panic(err)
		}

		w := httptest.NewRecorder()

		url := fmt.Sprintf("/api/users/user/%s", secondUser.ID)
		delUserReq, _ := http.NewRequest(http.MethodDelete, url, nil)
		delUserReq.Header.Set("Content-Type", "application/json")
		delUserReq.AddCookie(&http.Cookie{Name: accessTokenCookie.Name, Value: accessTokenCookie.Value})
		userRouter.ServeHTTP(w, delUserReq)

		var jsonResponse map[string]interface{}

		err = json.Unmarshal(w.Body.Bytes(), &jsonResponse)
		assert.Equal(t, jsonResponse["status"], "fail")

		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("UPDATE /api/users/user: fail with access token and empty update", func(t *testing.T) {
		firstUser := generateUser(random, authRouter, t)

		accessTokenCookie, err := loginUserGetAccessToken(t, firstUser.Password, firstUser.TelegramUserID, authRouter)

		if err != nil {
			panic(err)
		}

		w := httptest.NewRecorder()

		url := fmt.Sprintf("/api/users/user")
		updUserReq, _ := http.NewRequest(http.MethodPut, url, nil)
		updUserReq.Header.Set("Content-Type", "application/json")
		updUserReq.AddCookie(&http.Cookie{Name: accessTokenCookie.Name, Value: accessTokenCookie.Value})
		userRouter.ServeHTTP(w, updUserReq)

		assert.Equal(t, http.StatusBadGateway, w.Code)

	})

	t.Run("UPDATE /api/users/user: fail without access token", func(t *testing.T) {

		w := httptest.NewRecorder()

		url := fmt.Sprintf("/api/users/user")
		updUserReq, _ := http.NewRequest(http.MethodPut, url, nil)
		updUserReq.Header.Set("Content-Type", "application/json")
		userRouter.ServeHTTP(w, updUserReq)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("UPDATE /api/users/user: success with access token, update name", func(t *testing.T) {
		firstUser := generateUser(random, authRouter, t)

		accessTokenCookie, err := loginUserGetAccessToken(t, firstUser.Password, firstUser.TelegramUserID, authRouter)

		if err != nil {
			panic(err)
		}

		w := httptest.NewRecorder()

		payload := &models.UpdateUser{
			Name: fmt.Sprintf("%s-new", firstUser.Name),
		}

		jsonPayload, err := json.Marshal(payload)
		if err != nil {
			fmt.Println("Error marshaling payload:", err)
			return
		}

		url := fmt.Sprintf("/api/users/user")
		updUserReq, _ := http.NewRequest(http.MethodPut, url, bytes.NewBuffer(jsonPayload))
		updUserReq.Header.Set("Content-Type", "application/json")
		updUserReq.AddCookie(&http.Cookie{Name: accessTokenCookie.Name, Value: accessTokenCookie.Value})
		userRouter.ServeHTTP(w, updUserReq)

		assert.Equal(t, http.StatusOK, w.Code)

		var userResponse FindUserResponse

		err = json.Unmarshal(w.Body.Bytes(), &userResponse)
		assert.Nil(t, err)
		assert.Equal(t, userResponse.Status, "success")
		assert.Equal(t, userResponse.Data.Name, payload.Name)
	})

	t.Run("UPDATE /api/users/user: success with access token, update phone", func(t *testing.T) {
		firstUser := generateUser(random, authRouter, t)

		accessTokenCookie, err := loginUserGetAccessToken(t, firstUser.Password, firstUser.TelegramUserID, authRouter)

		if err != nil {
			panic(err)
		}

		w := httptest.NewRecorder()

		payload := &models.UpdateUser{
			Phone: "77000000101",
		}

		jsonPayload, err := json.Marshal(payload)
		if err != nil {
			fmt.Println("Error marshaling payload:", err)
			return
		}

		url := fmt.Sprintf("/api/users/user")
		updUserReq, _ := http.NewRequest(http.MethodPut, url, bytes.NewBuffer(jsonPayload))
		updUserReq.Header.Set("Content-Type", "application/json")
		updUserReq.AddCookie(&http.Cookie{Name: accessTokenCookie.Name, Value: accessTokenCookie.Value})
		userRouter.ServeHTTP(w, updUserReq)

		assert.Equal(t, http.StatusOK, w.Code)

		var userResponse FindUserResponse

		err = json.Unmarshal(w.Body.Bytes(), &userResponse)
		assert.Nil(t, err)
		assert.Equal(t, userResponse.Status, "success")
		assert.Equal(t, userResponse.Data.Phone, payload.Phone)
	})

	t.Run("UPDATE /api/users/user: success with access token, update phone", func(t *testing.T) {
		firstUser := generateUser(random, authRouter, t)

		accessTokenCookie, err := loginUserGetAccessToken(t, firstUser.Password, firstUser.TelegramUserID, authRouter)

		if err != nil {
			panic(err)
		}

		w := httptest.NewRecorder()

		payload := &models.UpdateUser{
			Avatar: "https://jollycontrarian.com/images/6/6c/Rickroll.jpg",
		}

		jsonPayload, err := json.Marshal(payload)
		if err != nil {
			fmt.Println("Error marshaling payload:", err)
			return
		}

		url := fmt.Sprintf("/api/users/user")
		updUserReq, _ := http.NewRequest(http.MethodPut, url, bytes.NewBuffer(jsonPayload))
		updUserReq.Header.Set("Content-Type", "application/json")
		updUserReq.AddCookie(&http.Cookie{Name: accessTokenCookie.Name, Value: accessTokenCookie.Value})
		userRouter.ServeHTTP(w, updUserReq)

		assert.Equal(t, http.StatusOK, w.Code)

		var userResponse FindUserResponse

		err = json.Unmarshal(w.Body.Bytes(), &userResponse)
		assert.Nil(t, err)
		assert.Equal(t, userResponse.Status, "success")
		assert.Equal(t, userResponse.Data.Avatar, payload.Avatar)
	})

}

package routes

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/ivegotanidea/golang-gorm-postgres/controllers"
	"github.com/ivegotanidea/golang-gorm-postgres/initializers"
	"github.com/ivegotanidea/golang-gorm-postgres/models"
	"github.com/stretchr/testify/assert"
	"log"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func SetupSCRouter(serviceController *controllers.ServiceController) *gin.Engine {
	r := gin.Default()

	serviceRouteController := NewRouteServiceController(*serviceController)

	api := r.Group("/api")
	serviceRouteController.ServiceRoute(api)

	return r
}

func SetupSCController() controllers.ServiceController {
	var err error
	config, err := initializers.LoadConfig("../.")
	if err != nil {
		log.Fatal("🚀 Could not load environment variables", err)
	}

	initializers.ConnectDB(&config)
	initializers.InitCasbin(&config)

	serviceController := controllers.NewServiceController(initializers.DB, config.ReviewUpdateLimitHours)
	serviceController.DB.Exec("CREATE EXTENSION IF NOT EXISTS \"uuid-ossp\"")

	if err := serviceController.DB.AutoMigrate(
		&models.HairColor{},
		&models.IntimateHairCut{},
		&models.Ethnos{},
		&models.BodyType{},
		&models.ProfileBodyArt{},
		&models.BodyArt{},
		&models.City{},
		&models.User{},
		&models.Profile{},
		&models.Service{},
		&models.Photo{},
		&models.ProfileOption{},
		&models.UserRating{},
		&models.UserTag{},
		&models.RatedUserTag{},
		&models.ProfileRating{},
		&models.ProfileTag{},
		&models.RatedProfileTag{}); err != nil {
		panic("failed to migrate database: " + err.Error())
	}

	return serviceController
}

func createProfile(t *testing.T, random *rand.Rand, cities []models.City, ethnos []models.Ethnos,
	profileTags []models.ProfileTag, bodyArts []models.BodyArt, bodyTypes []models.BodyType, hairColors []models.HairColor,
	intimateHairCuts []models.IntimateHairCut, accessTokenCookie *http.Cookie, profileRouter *gin.Engine,
	userID string) (CreateProfileResponse, error) {

	w := httptest.NewRecorder()

	payload := generateCreateProfileRequest(
		random, cities, ethnos, profileTags, bodyArts, bodyTypes, hairColors, intimateHairCuts)

	payload.BodyTypeID = nil

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		panic(err)
	}

	createProfileReq, _ := http.NewRequest("POST", "/api/profiles/", bytes.NewBuffer(jsonPayload))
	createProfileReq.AddCookie(&http.Cookie{Name: accessTokenCookie.Name, Value: accessTokenCookie.Value})
	createProfileReq.Header.Set("Content-Type", "application/json")

	profileRouter.ServeHTTP(w, createProfileReq)

	var profileResponse CreateProfileResponse
	err = json.Unmarshal(w.Body.Bytes(), &profileResponse)

	assert.Equal(t, profileResponse.Status, "success")
	assert.NotNil(t, profileResponse.Data.ID)
	checkProfilesMatch(t,
		userID, payload, profileResponse, true, false, false)

	assert.Equal(t, http.StatusCreated, w.Code)

	return profileResponse, nil
}

func createService(t *testing.T, clientID uuid.UUID, profileID uuid.UUID, profileOwnerID uuid.UUID,
	serviceRouter *gin.Engine, accessTokenCookie *http.Cookie, userTags []models.UserTag, profileTags []models.ProfileTag) (ServicesResponse, error) {

	payload := &models.CreateServiceRequest{
		ClientUserID:        clientID,
		ClientUserLatitude:  floatPtr(43.259769),
		ClientUserLongitude: floatPtr(76.935246),

		ProfileID:            profileID,
		ProfileOwnerID:       profileOwnerID,
		ProfileUserLatitude:  floatPtr(43.259879),
		ProfileUserLongitude: floatPtr(76.934604),
		ProfileRating: &models.CreateProfileRatingRequest{
			Review: "I like the service! It's very good",
			Score:  ptr(5),
			RatedProfileTags: []models.CreateRatedProfileTagRequest{
				{
					Type:  "like",
					TagID: profileTags[0].ID,
				},
				{
					Type:  "like",
					TagID: profileTags[1].ID,
				},
			},
		},
		UserRating: &models.CreateUserRatingRequest{
			Review: "I liked the client! He is very kind",
			Score:  ptr(5),
			RatedUserTags: []models.CreateRatedUserTagRequest{
				{
					Type:  "like",
					TagID: userTags[0].ID,
				},
				{
					Type:  "dislike",
					TagID: userTags[1].ID,
				},
			},
		},
	}

	return createServiceFromPayload(t, *payload, serviceRouter, accessTokenCookie)
}

func createServiceFromPayload(t *testing.T, payload models.CreateServiceRequest,
	serviceRouter *gin.Engine, accessTokenCookie *http.Cookie) (ServicesResponse, error) {

	w := httptest.NewRecorder()

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		fmt.Println("Error marshaling payload:", err)
		return ServicesResponse{}, err
	}

	createServiceReq, _ := http.NewRequest("POST", "/api/services/", bytes.NewBuffer(jsonPayload))
	createServiceReq.AddCookie(&http.Cookie{Name: accessTokenCookie.Name, Value: accessTokenCookie.Value})
	createServiceReq.Header.Set("Content-Type", "application/json")

	serviceRouter.ServeHTTP(w, createServiceReq)

	assert.Equal(t, http.StatusCreated, w.Code)

	w = httptest.NewRecorder()
	getServiceReq, _ := http.NewRequest("GET", fmt.Sprintf("/api/services/%s", payload.ProfileID), nil)
	getServiceReq.AddCookie(&http.Cookie{Name: accessTokenCookie.Name, Value: accessTokenCookie.Value})
	getServiceReq.Header.Set("Content-Type", "application/json")

	serviceRouter.ServeHTTP(w, getServiceReq)

	assert.Equal(t, http.StatusOK, w.Code)

	var servicesResponse ServicesResponse
	err = json.Unmarshal(w.Body.Bytes(), &servicesResponse)

	assert.NoError(t, err)
	assert.Equal(t, servicesResponse.Status, "success")
	assert.True(t, servicesResponse.Length >= 1)
	assert.Equal(t, servicesResponse.Data[0].TrustedDistance, true)
	assert.True(t, servicesResponse.Data[0].DistanceBetweenUsers <= 100)

	assert.NotNil(t, servicesResponse.Data[0].ID)

	assert.Equal(t, payload.ClientUserID, servicesResponse.Data[0].ClientUserID)
	assert.Equal(t, payload.ProfileID, servicesResponse.Data[0].ProfileID)

	return servicesResponse, nil
}

func TestServiceRoutes(t *testing.T) {

	ac := SetupAuthController()

	uc := SetupUCController()
	pc := SetupPCController()
	sc := SetupSCController()

	authRouter := SetupACRouter(&ac)
	userRouter := SetupUCRouter(&uc)
	profileRouter := SetupPCRouter(&pc)
	serviceRouter := SetupSCRouter(&sc)

	profileTags := populateProfileTags(*pc.DB)
	userTags := populateUserTags(*pc.DB)

	cities := populateCities(*pc.DB)

	// filters

	bodyTypes := populateBodyTypes(*pc.DB)

	ethnos := populateEthnos(*pc.DB)

	hairColors := populateHairColors(*pc.DB)

	intimateHairCuts := populateIntimateHairCuts(*pc.DB)

	bodyArts := populateBodyArts(*pc.DB)

	//createOwnerUser(profileController.DB)

	random := rand.New(rand.NewPCG(1, uint64(time.Now().Nanosecond())))

	t.Cleanup(func() {
		//utils.CleanupTestUsers(pc.DB)
		//utils.DropAllTables(pc.DB)
	})

	t.Run("POST /api/services/: fail without access_token", func(t *testing.T) {
		user := generateUser(random, authRouter, t, "")
		client := generateUser(random, authRouter, t, "")

		accessTokenCookie, _ := loginUserGetAccessToken(t, user.Password, user.TelegramUserID, authRouter)
		profile, _ := createProfile(t, random, cities, ethnos, profileTags, bodyArts, bodyTypes, hairColors,
			intimateHairCuts, accessTokenCookie, profileRouter, user.ID.String())

		log.Printf(profile.Status)

		payload := &models.CreateServiceRequest{
			ClientUserID:        client.ID,
			ClientUserLatitude:  floatPtr(43.259769),
			ClientUserLongitude: floatPtr(76.935246),

			ProfileID:            profile.Data.ID,
			ProfileUserLatitude:  floatPtr(43.259879),
			ProfileUserLongitude: floatPtr(76.934604),
		}

		w := httptest.NewRecorder()

		jsonPayload, err := json.Marshal(payload)
		if err != nil {
			fmt.Println("Error marshaling payload:", err)
			return
		}

		createServiceReq, _ := http.NewRequest("POST", "/api/services/", bytes.NewBuffer(jsonPayload))
		createServiceReq.Header.Set("Content-Type", "application/json")

		serviceRouter.ServeHTTP(w, createServiceReq)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("POST /api/services/: success with client's access_token", func(t *testing.T) {
		user := generateUser(random, authRouter, t, "")
		client := generateUser(random, authRouter, t, "")

		clientAccessTokenCookie, _ := loginUserGetAccessToken(t, client.Password, client.TelegramUserID, authRouter)

		accessTokenCookie, _ := loginUserGetAccessToken(t, user.Password, user.TelegramUserID, authRouter)
		profile, _ := createProfile(t, random, cities, ethnos, profileTags, bodyArts, bodyTypes, hairColors,
			intimateHairCuts, accessTokenCookie, profileRouter, user.ID.String())

		log.Printf(profile.Status)

		payload := &models.CreateServiceRequest{
			ClientUserID:        client.ID,
			ClientUserLatitude:  floatPtr(43.259769),
			ClientUserLongitude: floatPtr(76.935246),

			ProfileID:            profile.Data.ID,
			ProfileOwnerID:       profile.Data.UserID,
			ProfileUserLatitude:  floatPtr(43.259879),
			ProfileUserLongitude: floatPtr(76.934604),
		}

		w := httptest.NewRecorder()

		jsonPayload, err := json.Marshal(payload)
		if err != nil {
			fmt.Println("Error marshaling payload:", err)
			return
		}

		createServiceReq, _ := http.NewRequest("POST", "/api/services/", bytes.NewBuffer(jsonPayload))
		createServiceReq.AddCookie(&http.Cookie{Name: clientAccessTokenCookie.Name, Value: clientAccessTokenCookie.Value})
		createServiceReq.Header.Set("Content-Type", "application/json")

		serviceRouter.ServeHTTP(w, createServiceReq)

		assert.Equal(t, http.StatusCreated, w.Code)
	})

	t.Run("POST /api/services/: success with profile author's access_token", func(t *testing.T) {
		user := generateUser(random, authRouter, t, "")
		client := generateUser(random, authRouter, t, "")

		accessTokenCookie, _ := loginUserGetAccessToken(t, user.Password, user.TelegramUserID, authRouter)
		profile, _ := createProfile(t, random, cities, ethnos, profileTags, bodyArts, bodyTypes, hairColors,
			intimateHairCuts, accessTokenCookie, profileRouter, user.ID.String())

		payload := &models.CreateServiceRequest{
			ClientUserID:        client.ID,
			ClientUserLatitude:  floatPtr(43.259769),
			ClientUserLongitude: floatPtr(76.935246),

			ProfileID:            profile.Data.ID,
			ProfileOwnerID:       profile.Data.UserID,
			ProfileUserLatitude:  floatPtr(43.259879),
			ProfileUserLongitude: floatPtr(76.934604),
		}

		w := httptest.NewRecorder()

		jsonPayload, err := json.Marshal(payload)
		if err != nil {
			fmt.Println("Error marshaling payload:", err)
			return
		}

		createServiceReq, _ := http.NewRequest("POST", "/api/services/", bytes.NewBuffer(jsonPayload))
		createServiceReq.AddCookie(&http.Cookie{Name: accessTokenCookie.Name, Value: accessTokenCookie.Value})
		createServiceReq.Header.Set("Content-Type", "application/json")

		serviceRouter.ServeHTTP(w, createServiceReq)

		assert.Equal(t, http.StatusCreated, w.Code)

		w = httptest.NewRecorder()
		getServiceReq, _ := http.NewRequest("GET", fmt.Sprintf("/api/services/%s", profile.Data.ID.String()), bytes.NewBuffer(jsonPayload))
		getServiceReq.AddCookie(&http.Cookie{Name: accessTokenCookie.Name, Value: accessTokenCookie.Value})
		getServiceReq.Header.Set("Content-Type", "application/json")

		serviceRouter.ServeHTTP(w, getServiceReq)

		assert.Equal(t, http.StatusOK, w.Code)

		var servicesResponse ServicesResponse
		err = json.Unmarshal(w.Body.Bytes(), &servicesResponse)

		assert.NoError(t, err)
		assert.Equal(t, servicesResponse.Status, "success")
		assert.True(t, servicesResponse.Length >= 1)
		assert.Equal(t, servicesResponse.Data[0].TrustedDistance, true)
		assert.True(t, servicesResponse.Data[0].DistanceBetweenUsers <= 100)

		assert.NotNil(t, servicesResponse.Data[0].ID)

		assert.Nil(t, servicesResponse.Data[0].ClientUserRating)
		assert.Nil(t, servicesResponse.Data[0].ClientUserRatingID)
		assert.Nil(t, servicesResponse.Data[0].ProfileRatingID)
		assert.Nil(t, servicesResponse.Data[0].ProfileRating)

		assert.Equal(t, client.ID, servicesResponse.Data[0].ClientUserID)
		assert.Equal(t, profile.Data.ID, servicesResponse.Data[0].ProfileID)

	})

	t.Run("GET /api/services/:profileID: basic user can only see score,  not review's text or tags", func(t *testing.T) {

		profileOwner := generateUser(random, authRouter, t, "")
		clientUser := generateUser(random, authRouter, t, "")

		accessTokenCookie, _ := loginUserGetAccessToken(t, profileOwner.Password, profileOwner.TelegramUserID, authRouter)
		profile, _ := createProfile(t, random, cities, ethnos, profileTags, bodyArts, bodyTypes, hairColors,
			intimateHairCuts, accessTokenCookie, profileRouter, profileOwner.ID.String())

		service, _ := createService(t, clientUser.ID, profile.Data.ID,
			profileOwner.ID, serviceRouter, accessTokenCookie, userTags, profileTags)

		assert.NotNil(t, service)

		basicUser := generateUser(random, authRouter, t, "")
		basicUserAccessTokenCookie, _ := loginUserGetAccessToken(t, basicUser.Password, basicUser.TelegramUserID, authRouter)

		w := httptest.NewRecorder()
		getServiceReq, _ := http.NewRequest("GET", fmt.Sprintf("/api/services/%s", profile.Data.ID.String()), nil)
		getServiceReq.AddCookie(&http.Cookie{Name: basicUserAccessTokenCookie.Name, Value: basicUserAccessTokenCookie.Value})
		getServiceReq.Header.Set("Content-Type", "application/json")

		serviceRouter.ServeHTTP(w, getServiceReq)

		assert.Equal(t, http.StatusOK, w.Code)

		var servicesResponse ServicesResponse
		err := json.Unmarshal(w.Body.Bytes(), &servicesResponse)

		assert.NoError(t, err)

		assert.Equal(t, servicesResponse.Status, "success")
		assert.True(t, servicesResponse.Length == 1)
		assert.NotNil(t, servicesResponse.Data[0].ProfileRatingID)
		assert.NotNil(t, servicesResponse.Data[0].ProfileOwnerID)
		assert.Nil(t, servicesResponse.Data[0].ProfileRating.RatedProfileTags)
		assert.True(t, servicesResponse.Data[0].ProfileRating.ReviewTextVisible)
		assert.Empty(t, servicesResponse.Data[0].ProfileRating.Review)
		assert.Equal(t, servicesResponse.Data[0].ProfileRating.Score, service.Data[0].ProfileRating.Score)

		assert.NotNil(t, servicesResponse.Data[0].ClientUserID)
		assert.NotNil(t, servicesResponse.Data[0].ClientUserRatingID)
		assert.Nil(t, servicesResponse.Data[0].ClientUserRating)

		assert.Empty(t, servicesResponse.Data[0].ProfileUserLon)
		assert.Empty(t, servicesResponse.Data[0].ProfileUserLat)
		assert.Empty(t, servicesResponse.Data[0].ClientUserLon)
		assert.Empty(t, servicesResponse.Data[0].ClientUserLat)

		assert.NotNil(t, servicesResponse.Data[0].CreatedAt)
		assert.NotNil(t, servicesResponse.Data[0].UpdatedAt)
		assert.NotNil(t, servicesResponse.Data[0].UpdatedBy)

	})

	t.Run("GET /api/services/:profileID: expert user can only see score,  not review's text or tags", func(t *testing.T) {

		profileOwner := generateUser(random, authRouter, t, "")
		clientUser := generateUser(random, authRouter, t, "")

		accessTokenCookie, _ := loginUserGetAccessToken(t, profileOwner.Password, profileOwner.TelegramUserID, authRouter)
		profile, _ := createProfile(t, random, cities, ethnos, profileTags, bodyArts, bodyTypes, hairColors,
			intimateHairCuts, accessTokenCookie, profileRouter, profileOwner.ID.String())

		payload := &models.CreateServiceRequest{
			ClientUserID:        clientUser.ID,
			ClientUserLatitude:  floatPtr(43.259769),
			ClientUserLongitude: floatPtr(76.935246),

			ProfileID:            profile.Data.ID,
			ProfileOwnerID:       profileOwner.ID,
			ProfileUserLatitude:  floatPtr(43.259879),
			ProfileUserLongitude: floatPtr(76.934604),
			ProfileRating: &models.CreateProfileRatingRequest{
				Review: "I like the service! It's very good",
				Score:  ptr(5),
				RatedProfileTags: []models.CreateRatedProfileTagRequest{
					{
						Type:  "like",
						TagID: profileTags[0].ID,
					},
					{
						Type:  "like",
						TagID: profileTags[1].ID,
					},
				},
			},
			UserRating: &models.CreateUserRatingRequest{
				Review: "I liked the client! He is very kind",
				Score:  ptr(5),
				RatedUserTags: []models.CreateRatedUserTagRequest{
					{
						Type:  "like",
						TagID: userTags[0].ID,
					},
					{
						Type:  "dislike",
						TagID: userTags[1].ID,
					},
				},
			},
		}

		service, _ := createServiceFromPayload(t, *payload, serviceRouter, accessTokenCookie)

		assert.NotNil(t, service)

		basicUser := generateUser(random, authRouter, t, "expert")
		basicUserAccessTokenCookie, _ := loginUserGetAccessToken(t, basicUser.Password, basicUser.TelegramUserID, authRouter)

		w := httptest.NewRecorder()
		getServiceReq, _ := http.NewRequest("GET", fmt.Sprintf("/api/services/%s", profile.Data.ID.String()), nil)
		getServiceReq.AddCookie(&http.Cookie{Name: basicUserAccessTokenCookie.Name, Value: basicUserAccessTokenCookie.Value})
		getServiceReq.Header.Set("Content-Type", "application/json")

		serviceRouter.ServeHTTP(w, getServiceReq)

		assert.Equal(t, http.StatusOK, w.Code)

		var servicesResponse ServicesResponse
		err := json.Unmarshal(w.Body.Bytes(), &servicesResponse)

		assert.NoError(t, err)

		assert.Equal(t, servicesResponse.Status, "success")
		assert.True(t, servicesResponse.Length == 1)
		assert.NotNil(t, servicesResponse.Data[0].ProfileRatingID)
		assert.NotNil(t, servicesResponse.Data[0].ProfileOwnerID)
		assert.Nil(t, servicesResponse.Data[0].ProfileRating.RatedProfileTags)
		assert.False(t, servicesResponse.Data[0].ProfileRating.ReviewTextVisible)
		assert.Equal(t, servicesResponse.Data[0].ProfileRating.Review, payload.ProfileRating.Review)
		assert.Equal(t, servicesResponse.Data[0].ProfileRating.Score, payload.ProfileRating.Score)

		assert.NotNil(t, servicesResponse.Data[0].ClientUserID)
		assert.NotNil(t, servicesResponse.Data[0].ClientUserRatingID)
		assert.NotNil(t, servicesResponse.Data[0].ClientUserRating)

		assert.Nil(t, servicesResponse.Data[0].ClientUserRating.RatedUserTags)
		assert.True(t, servicesResponse.Data[0].ClientUserRating.ReviewTextVisible)
		assert.Equal(t, servicesResponse.Data[0].ClientUserRating.Score, payload.UserRating.Score)

		assert.Empty(t, servicesResponse.Data[0].ProfileUserLon)
		assert.Empty(t, servicesResponse.Data[0].ProfileUserLat)
		assert.Empty(t, servicesResponse.Data[0].ClientUserLon)
		assert.Empty(t, servicesResponse.Data[0].ClientUserLat)

		assert.NotNil(t, servicesResponse.Data[0].CreatedAt)
		assert.NotNil(t, servicesResponse.Data[0].UpdatedAt)
		assert.NotNil(t, servicesResponse.Data[0].UpdatedBy)

	})

	t.Run("GET /api/services/:profileID: guru user can only see score,  not review's text or tags", func(t *testing.T) {

		profileOwner := generateUser(random, authRouter, t, "")
		clientUser := generateUser(random, authRouter, t, "")

		accessTokenCookie, _ := loginUserGetAccessToken(t, profileOwner.Password, profileOwner.TelegramUserID, authRouter)
		profile, _ := createProfile(t, random, cities, ethnos, profileTags, bodyArts, bodyTypes, hairColors,
			intimateHairCuts, accessTokenCookie, profileRouter, profileOwner.ID.String())

		payload := &models.CreateServiceRequest{
			ClientUserID:        clientUser.ID,
			ClientUserLatitude:  floatPtr(43.259769),
			ClientUserLongitude: floatPtr(76.935246),

			ProfileID:            profile.Data.ID,
			ProfileOwnerID:       profileOwner.ID,
			ProfileUserLatitude:  floatPtr(43.259879),
			ProfileUserLongitude: floatPtr(76.934604),
			ProfileRating: &models.CreateProfileRatingRequest{
				Review: "I like the service! It's very good",
				Score:  ptr(5),
				RatedProfileTags: []models.CreateRatedProfileTagRequest{
					{
						Type:  "like",
						TagID: profileTags[0].ID,
					},
					{
						Type:  "like",
						TagID: profileTags[1].ID,
					},
				},
			},
			UserRating: &models.CreateUserRatingRequest{
				Review: "I liked the client! He is very kind",
				Score:  ptr(5),
				RatedUserTags: []models.CreateRatedUserTagRequest{
					{
						Type:  "like",
						TagID: userTags[0].ID,
					},
					{
						Type:  "dislike",
						TagID: userTags[1].ID,
					},
				},
			},
		}

		service, _ := createServiceFromPayload(t, *payload, serviceRouter, accessTokenCookie)

		assert.NotNil(t, service)

		basicUser := generateUser(random, authRouter, t, "guru")
		basicUserAccessTokenCookie, _ := loginUserGetAccessToken(t, basicUser.Password, basicUser.TelegramUserID, authRouter)

		w := httptest.NewRecorder()
		getServiceReq, _ := http.NewRequest("GET", fmt.Sprintf("/api/services/%s", profile.Data.ID.String()), nil)
		getServiceReq.AddCookie(&http.Cookie{Name: basicUserAccessTokenCookie.Name, Value: basicUserAccessTokenCookie.Value})
		getServiceReq.Header.Set("Content-Type", "application/json")

		serviceRouter.ServeHTTP(w, getServiceReq)

		assert.Equal(t, http.StatusOK, w.Code)

		var servicesResponse ServicesResponse
		err := json.Unmarshal(w.Body.Bytes(), &servicesResponse)

		assert.NoError(t, err)

		assert.Equal(t, servicesResponse.Status, "success")
		assert.True(t, servicesResponse.Length == 1)
		assert.NotNil(t, servicesResponse.Data[0].ProfileRatingID)
		assert.NotNil(t, servicesResponse.Data[0].ProfileOwnerID)
		assert.NotNil(t, servicesResponse.Data[0].ProfileRating.RatedProfileTags)
		assert.True(t, servicesResponse.Data[0].ProfileRating.ReviewTextVisible)
		assert.Equal(t, servicesResponse.Data[0].ProfileRating.Review, payload.ProfileRating.Review)
		assert.Equal(t, servicesResponse.Data[0].ProfileRating.Score, payload.ProfileRating.Score)

		assert.NotNil(t, servicesResponse.Data[0].ClientUserID)
		assert.NotNil(t, servicesResponse.Data[0].ClientUserRatingID)
		assert.NotNil(t, servicesResponse.Data[0].ClientUserRating)

		assert.NotNil(t, servicesResponse.Data[0].ClientUserRating.RatedUserTags)
		assert.True(t, servicesResponse.Data[0].ClientUserRating.ReviewTextVisible)
		assert.Equal(t, servicesResponse.Data[0].ClientUserRating.Score, payload.UserRating.Score)

		assert.Empty(t, servicesResponse.Data[0].ProfileUserLon)
		assert.Empty(t, servicesResponse.Data[0].ProfileUserLat)
		assert.Empty(t, servicesResponse.Data[0].ClientUserLon)
		assert.Empty(t, servicesResponse.Data[0].ClientUserLat)

		assert.NotNil(t, servicesResponse.Data[0].CreatedAt)
		assert.NotNil(t, servicesResponse.Data[0].UpdatedAt)
		assert.NotNil(t, servicesResponse.Data[0].UpdatedBy)

	})

	t.Run("GET /api/services/:profileID: basic can't list services", func(t *testing.T) {

		profileOwner := generateUser(random, authRouter, t, "")
		clientUser := generateUser(random, authRouter, t, "")

		accessTokenCookie, _ := loginUserGetAccessToken(t, profileOwner.Password, profileOwner.TelegramUserID, authRouter)
		profile, _ := createProfile(t, random, cities, ethnos, profileTags, bodyArts, bodyTypes, hairColors,
			intimateHairCuts, accessTokenCookie, profileRouter, profileOwner.ID.String())

		payload := &models.CreateServiceRequest{
			ClientUserID:        clientUser.ID,
			ClientUserLatitude:  floatPtr(43.259769),
			ClientUserLongitude: floatPtr(76.935246),

			ProfileID:            profile.Data.ID,
			ProfileOwnerID:       profileOwner.ID,
			ProfileUserLatitude:  floatPtr(43.259879),
			ProfileUserLongitude: floatPtr(76.934604),
			ProfileRating: &models.CreateProfileRatingRequest{
				Review: "I like the service! It's very good",
				Score:  ptr(5),
				RatedProfileTags: []models.CreateRatedProfileTagRequest{
					{
						Type:  "like",
						TagID: profileTags[0].ID,
					},
					{
						Type:  "like",
						TagID: profileTags[1].ID,
					},
				},
			},
			UserRating: &models.CreateUserRatingRequest{
				Review: "I liked the client! He is very kind",
				Score:  ptr(5),
				RatedUserTags: []models.CreateRatedUserTagRequest{
					{
						Type:  "like",
						TagID: userTags[0].ID,
					},
					{
						Type:  "dislike",
						TagID: userTags[1].ID,
					},
				},
			},
		}

		service, _ := createServiceFromPayload(t, *payload, serviceRouter, accessTokenCookie)

		assert.NotNil(t, service)

		basicUser := generateUser(random, authRouter, t, "basic")
		basicUserAccessTokenCookie, _ := loginUserGetAccessToken(t, basicUser.Password, basicUser.TelegramUserID, authRouter)

		w := httptest.NewRecorder()
		listServicesReq, _ := http.NewRequest("GET", "/api/services/all", nil)
		listServicesReq.AddCookie(&http.Cookie{Name: basicUserAccessTokenCookie.Name, Value: basicUserAccessTokenCookie.Value})
		listServicesReq.Header.Set("Content-Type", "application/json")

		serviceRouter.ServeHTTP(w, listServicesReq)

		assert.Equal(t, http.StatusForbidden, w.Code)

	})

	t.Run("GET /api/services/:profileID: expert can't list services", func(t *testing.T) {

		profileOwner := generateUser(random, authRouter, t, "")
		clientUser := generateUser(random, authRouter, t, "")

		accessTokenCookie, _ := loginUserGetAccessToken(t, profileOwner.Password, profileOwner.TelegramUserID, authRouter)
		profile, _ := createProfile(t, random, cities, ethnos, profileTags, bodyArts, bodyTypes, hairColors,
			intimateHairCuts, accessTokenCookie, profileRouter, profileOwner.ID.String())

		payload := &models.CreateServiceRequest{
			ClientUserID:        clientUser.ID,
			ClientUserLatitude:  floatPtr(43.259769),
			ClientUserLongitude: floatPtr(76.935246),

			ProfileID:            profile.Data.ID,
			ProfileOwnerID:       profileOwner.ID,
			ProfileUserLatitude:  floatPtr(43.259879),
			ProfileUserLongitude: floatPtr(76.934604),
			ProfileRating: &models.CreateProfileRatingRequest{
				Review: "I like the service! It's very good",
				Score:  ptr(5),
				RatedProfileTags: []models.CreateRatedProfileTagRequest{
					{
						Type:  "like",
						TagID: profileTags[0].ID,
					},
					{
						Type:  "like",
						TagID: profileTags[1].ID,
					},
				},
			},
			UserRating: &models.CreateUserRatingRequest{
				Review: "I liked the client! He is very kind",
				Score:  ptr(5),
				RatedUserTags: []models.CreateRatedUserTagRequest{
					{
						Type:  "like",
						TagID: userTags[0].ID,
					},
					{
						Type:  "dislike",
						TagID: userTags[1].ID,
					},
				},
			},
		}

		service, _ := createServiceFromPayload(t, *payload, serviceRouter, accessTokenCookie)

		assert.NotNil(t, service)

		basicUser := generateUser(random, authRouter, t, "expert")
		basicUserAccessTokenCookie, _ := loginUserGetAccessToken(t, basicUser.Password, basicUser.TelegramUserID, authRouter)

		w := httptest.NewRecorder()
		listServicesReq, _ := http.NewRequest("GET", "/api/services/all", nil)
		listServicesReq.AddCookie(&http.Cookie{Name: basicUserAccessTokenCookie.Name, Value: basicUserAccessTokenCookie.Value})
		listServicesReq.Header.Set("Content-Type", "application/json")

		serviceRouter.ServeHTTP(w, listServicesReq)

		assert.Equal(t, http.StatusForbidden, w.Code)

	})

	t.Run("GET /api/services/:profileID: guru can list services", func(t *testing.T) {

		profileOwner := generateUser(random, authRouter, t, "")
		clientUser := generateUser(random, authRouter, t, "")

		accessTokenCookie, _ := loginUserGetAccessToken(t, profileOwner.Password, profileOwner.TelegramUserID, authRouter)
		profile, _ := createProfile(t, random, cities, ethnos, profileTags, bodyArts, bodyTypes, hairColors,
			intimateHairCuts, accessTokenCookie, profileRouter, profileOwner.ID.String())

		payload := &models.CreateServiceRequest{
			ClientUserID:        clientUser.ID,
			ClientUserLatitude:  floatPtr(43.259769),
			ClientUserLongitude: floatPtr(76.935246),

			ProfileID:            profile.Data.ID,
			ProfileOwnerID:       profileOwner.ID,
			ProfileUserLatitude:  floatPtr(43.259879),
			ProfileUserLongitude: floatPtr(76.934604),
			ProfileRating: &models.CreateProfileRatingRequest{
				Review: "I like the service! It's very good",
				Score:  ptr(5),
				RatedProfileTags: []models.CreateRatedProfileTagRequest{
					{
						Type:  "like",
						TagID: profileTags[0].ID,
					},
					{
						Type:  "like",
						TagID: profileTags[1].ID,
					},
				},
			},
			UserRating: &models.CreateUserRatingRequest{
				Review: "I liked the client! He is very kind",
				Score:  ptr(5),
				RatedUserTags: []models.CreateRatedUserTagRequest{
					{
						Type:  "like",
						TagID: userTags[0].ID,
					},
					{
						Type:  "dislike",
						TagID: userTags[1].ID,
					},
				},
			},
		}

		service, _ := createServiceFromPayload(t, *payload, serviceRouter, accessTokenCookie)

		assert.NotNil(t, service)

		basicUser := generateUser(random, authRouter, t, "guru")
		basicUserAccessTokenCookie, _ := loginUserGetAccessToken(t, basicUser.Password, basicUser.TelegramUserID, authRouter)

		w := httptest.NewRecorder()
		listServicesReq, _ := http.NewRequest("GET", "/api/services/all", nil)
		listServicesReq.AddCookie(&http.Cookie{Name: basicUserAccessTokenCookie.Name, Value: basicUserAccessTokenCookie.Value})
		listServicesReq.Header.Set("Content-Type", "application/json")

		serviceRouter.ServeHTTP(w, listServicesReq)

		assert.Equal(t, http.StatusOK, w.Code)

		var servicesResponse ServicesResponse
		err := json.Unmarshal(w.Body.Bytes(), &servicesResponse)

		assert.NoError(t, err)
		assert.True(t, servicesResponse.Length >= len(service.Data))
	})

	t.Run("GET /api/services/:profileID: moderator can list services", func(t *testing.T) {

		profileOwner := generateUser(random, authRouter, t, "")
		clientUser := generateUser(random, authRouter, t, "")

		accessTokenCookie, _ := loginUserGetAccessToken(t, profileOwner.Password, profileOwner.TelegramUserID, authRouter)
		profile, _ := createProfile(t, random, cities, ethnos, profileTags, bodyArts, bodyTypes, hairColors,
			intimateHairCuts, accessTokenCookie, profileRouter, profileOwner.ID.String())

		payload := &models.CreateServiceRequest{
			ClientUserID:        clientUser.ID,
			ClientUserLatitude:  floatPtr(43.259769),
			ClientUserLongitude: floatPtr(76.935246),

			ProfileID:            profile.Data.ID,
			ProfileOwnerID:       profileOwner.ID,
			ProfileUserLatitude:  floatPtr(43.259879),
			ProfileUserLongitude: floatPtr(76.934604),
			ProfileRating: &models.CreateProfileRatingRequest{
				Review: "I like the service! It's very good",
				Score:  ptr(5),
				RatedProfileTags: []models.CreateRatedProfileTagRequest{
					{
						Type:  "like",
						TagID: profileTags[0].ID,
					},
					{
						Type:  "like",
						TagID: profileTags[1].ID,
					},
				},
			},
			UserRating: &models.CreateUserRatingRequest{
				Review: "I liked the client! He is very kind",
				Score:  ptr(5),
				RatedUserTags: []models.CreateRatedUserTagRequest{
					{
						Type:  "like",
						TagID: userTags[0].ID,
					},
					{
						Type:  "dislike",
						TagID: userTags[1].ID,
					},
				},
			},
		}

		service, _ := createServiceFromPayload(t, *payload, serviceRouter, accessTokenCookie)

		assert.NotNil(t, service)

		basicUser := generateUser(random, authRouter, t, "guru")
		basicUserPassword := basicUser.Password
		basicUser = assignRole(initializers.DB, t, authRouter, userRouter, basicUser.ID.String(), "moderator")

		basicUserAccessTokenCookie, _ := loginUserGetAccessToken(t, basicUserPassword, basicUser.TelegramUserID, authRouter)

		w := httptest.NewRecorder()
		listServicesReq, _ := http.NewRequest("GET", "/api/services/all", nil)
		listServicesReq.AddCookie(&http.Cookie{Name: basicUserAccessTokenCookie.Name, Value: basicUserAccessTokenCookie.Value})
		listServicesReq.Header.Set("Content-Type", "application/json")

		serviceRouter.ServeHTTP(w, listServicesReq)

		assert.Equal(t, http.StatusOK, w.Code)

		var servicesResponse ServicesResponse
		err := json.Unmarshal(w.Body.Bytes(), &servicesResponse)

		assert.NoError(t, err)
		assert.True(t, servicesResponse.Length >= len(service.Data))
	})
}

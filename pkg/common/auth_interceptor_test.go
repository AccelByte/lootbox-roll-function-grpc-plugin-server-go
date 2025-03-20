// Copyright (c) 2022 AccelByte Inc. All Rights Reserved.
// This is licensed software from AccelByte Inc, for limitations
// and restrictions contact your company contract manager.

package common

import (
	"github.com/AccelByte/accelbyte-go-sdk/services-api/pkg/factory"
	"github.com/AccelByte/accelbyte-go-sdk/services-api/pkg/service/iam"
	sdkAuth "github.com/AccelByte/accelbyte-go-sdk/services-api/pkg/utils/auth"
	"time"

	"github.com/stretchr/testify/assert"

	"testing"
)

func TestTokenValidator_ValidateToken(t *testing.T) {
	t.Skip() // "TODO: mock and remove hardcoded client id and secret"

	// Arrange
	namespace := GetEnv("AB_NAMESPACE", "accelbyte")
	clientId := ""
	clientSecret := ""
	configRepo := sdkAuth.DefaultConfigRepositoryImpl()
	tokenRepo := sdkAuth.DefaultTokenRepositoryImpl()
	authService := iam.OAuth20Service{
		Client:           factory.NewIamClient(configRepo),
		ConfigRepository: configRepo,
		TokenRepository:  tokenRepo,
	}

	err := authService.LoginClient(&clientId, &clientSecret)
	if err != nil {
		assert.Fail(t, err.Error())

		return
	}

	accessToken, err := authService.GetToken()
	if err != nil {
		assert.Fail(t, err.Error())

		return
	}

	Validator = NewTokenValidator(authService, time.Duration(600)*time.Second, true)
	Validator.Initialize()

	// Act
	err = authService.Validate(accessToken, nil, &namespace, nil)

	// Assert
	assert.Nil(t, err)
}

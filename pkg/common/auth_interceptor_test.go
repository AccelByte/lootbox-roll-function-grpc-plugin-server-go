// Copyright (c) 2022 AccelByte Inc. All Rights Reserved.
// This is licensed software from AccelByte Inc, for limitations
// and restrictions contact your company contract manager.

package common

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/AccelByte/accelbyte-go-sdk/iam-sdk/pkg/iamclient/o_auth2_0"
	"github.com/AccelByte/accelbyte-go-sdk/services-api/pkg/factory"
	"github.com/AccelByte/accelbyte-go-sdk/services-api/pkg/service/iam"
	sdkAuth "github.com/AccelByte/accelbyte-go-sdk/services-api/pkg/utils/auth"
	"github.com/stretchr/testify/assert"

	"google.golang.org/grpc/metadata"
)

func TestUnaryAuthServerIntercept(t *testing.T) {
	t.Skip() // "TODO: mock the OAuth"

	configRepo := sdkAuth.DefaultConfigRepositoryImpl()
	tokenRepo := sdkAuth.DefaultTokenRepositoryImpl()
	OAuth = &iam.OAuth20Service{
		Client:           factory.NewIamClient(configRepo),
		ConfigRepository: configRepo,
		TokenRepository:  tokenRepo,
	}

	extendNamespace := os.Getenv("AB_NAMESPACE")
	token, errToken := OAuth.TokenGrantV3Short(&o_auth2_0.TokenGrantV3Params{
		ExtendNamespace: &extendNamespace,
		GrantType:       o_auth2_0.TokenGrantV3UrnIetfParamsOauthGrantTypeExtendClientCredentialsConstant,
	})
	assert.Nil(t, errToken)

	OAuth.SetLocalValidation(true)

	md := map[string]string{
		"authorization": fmt.Sprintf("Bearer %s", *token.AccessToken),
	}
	ctx := metadata.NewIncomingContext(context.Background(), metadata.New(md))

	req := struct{}{}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return req, nil
	}

	// test
	res, err := UnaryAuthServerIntercept(ctx, req, nil, handler)
	assert.Nil(t, err)
	assert.Equal(t, req, res)
}

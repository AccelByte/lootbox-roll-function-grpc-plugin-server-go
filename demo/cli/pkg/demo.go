// Copyright (c) 2023 AccelByte Inc. All Rights Reserved.
// This is licensed software from AccelByte Inc, for limitations
// and restrictions contact your company contract manager.

package lootboxrolldemo

import (
	"fmt"

	"github.com/AccelByte/accelbyte-go-sdk/platform-sdk/pkg/platformclient/catalog_changes"
	"github.com/AccelByte/accelbyte-go-sdk/platform-sdk/pkg/platformclient/category"
	"github.com/AccelByte/accelbyte-go-sdk/platform-sdk/pkg/platformclient/currency"
	"github.com/AccelByte/accelbyte-go-sdk/platform-sdk/pkg/platformclient/entitlement"
	"github.com/AccelByte/accelbyte-go-sdk/platform-sdk/pkg/platformclient/item"
	"github.com/AccelByte/accelbyte-go-sdk/platform-sdk/pkg/platformclient/service_plugin_config"
	"github.com/AccelByte/accelbyte-go-sdk/platform-sdk/pkg/platformclient/store"
	"github.com/AccelByte/accelbyte-go-sdk/platform-sdk/pkg/platformclientmodels"
	"github.com/AccelByte/accelbyte-go-sdk/services-api/pkg/factory"
	"github.com/AccelByte/accelbyte-go-sdk/services-api/pkg/repository"
	"github.com/AccelByte/accelbyte-go-sdk/services-api/pkg/service/platform"
	"github.com/pkg/errors"
)

const ALPHA_CHARS = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

var (
	abStoreName = "GO Lootbox Plugin Demo Store"
	abStoreDesc = "GO Description for Lootbox grpc plugin demo store"
)

var errEmptyStoreID = errors.New("error empty store id, createStore first")

type PlatformDataUnit struct {
	CLIConfig    *Config
	ConfigRepo   repository.ConfigRepository
	TokenRepo    repository.TokenRepository
	storeID      string
	CurrencyCode string
}

func (p *PlatformDataUnit) SetPlatformServiceGrpcTarget() error {
	servicePluginCfgWrapper := platform.ServicePluginConfigService{
		Client:           factory.NewPlatformClient(p.ConfigRepo),
		ConfigRepository: p.ConfigRepo,
		TokenRepository:  p.TokenRepo,
	}

	if p.CLIConfig.GRPCServerURL != "" {
		fmt.Printf("(Custom Host: %s) ", p.CLIConfig.GRPCServerURL)

		_, err := servicePluginCfgWrapper.UpdateLootBoxPluginConfigShort(&service_plugin_config.UpdateLootBoxPluginConfigParams{
			Namespace: p.CLIConfig.ABNamespace,
			Body: &platformclientmodels.LootBoxPluginConfigUpdate{
				ExtendType: Ptr(platformclientmodels.LootBoxPluginConfigUpdateExtendTypeCUSTOM),
				CustomConfig: &platformclientmodels.BaseCustomConfig{
					ConnectionType:    Ptr(platformclientmodels.BaseCustomConfigConnectionTypeINSECURE),
					GrpcServerAddress: Ptr(p.CLIConfig.GRPCServerURL),
				},
			},
		})

		return err
	}

	if p.CLIConfig.ExtendAppName != "" {
		fmt.Printf("(Extend App: %s) ", p.CLIConfig.ExtendAppName)

		_, err := servicePluginCfgWrapper.UpdateLootBoxPluginConfigShort(&service_plugin_config.UpdateLootBoxPluginConfigParams{
			Namespace: p.CLIConfig.ABNamespace,
			Body: &platformclientmodels.LootBoxPluginConfigUpdate{
				ExtendType: Ptr(platformclientmodels.LootBoxPluginConfigUpdateExtendTypeAPP),
				AppConfig: &platformclientmodels.AppConfig{
					AppName: Ptr(p.CLIConfig.ExtendAppName),
				},
			},
		})

		return err
	}

	return nil
}

func (p *PlatformDataUnit) CreateStore(doPublish bool) error {
	storeWrapper := platform.StoreService{
		Client:           factory.NewPlatformClient(p.ConfigRepo),
		ConfigRepository: p.ConfigRepo,
		TokenRepository:  p.TokenRepo,
	}

	// Clean up existing stores
	storeInfo, err := storeWrapper.ListStoresShort(&store.ListStoresParams{
		Namespace: p.CLIConfig.ABNamespace,
	})
	if err != nil {
		return err
	}
	for _, s := range storeInfo {
		if !Val(s.Published) {
			_, _ = storeWrapper.DeleteStoreShort(&store.DeleteStoreParams{
				Namespace: p.CLIConfig.ABNamespace,
				StoreID:   Val(s.StoreID),
			})
		}
	}

	// Create and publish new store
	newStore, err := storeWrapper.CreateStoreShort(&store.CreateStoreParams{
		Namespace: p.CLIConfig.ABNamespace,
		Body: &platformclientmodels.StoreCreate{
			DefaultLanguage:    "en",
			DefaultRegion:      "US",
			Description:        abStoreDesc,
			SupportedLanguages: []string{"en"},
			SupportedRegions:   []string{"US"},
			Title:              &abStoreName,
		},
	})
	if err != nil {
		return fmt.Errorf("could not create new store: %w", err)
	}

	p.storeID = Val(newStore.StoreID)
	if doPublish {
		err = p.publishStoreChange()
		if err != nil {
			return err
		}
	}

	return nil
}

func (p *PlatformDataUnit) publishStoreChange() error {
	catalogWrapper := platform.CatalogChangesService{
		Client:           factory.NewPlatformClient(p.ConfigRepo),
		ConfigRepository: p.ConfigRepo,
		TokenRepository:  p.TokenRepo,
	}
	_, err := catalogWrapper.PublishAllShort(&catalog_changes.PublishAllParams{
		Namespace: p.CLIConfig.ABNamespace,
		StoreID:   p.storeID,
	})
	if err != nil {
		return fmt.Errorf("could not publish store: %w", err)
	}

	return nil
}

func (p *PlatformDataUnit) CreateCategory(categoryPath string, doPublish bool) error {
	if p.storeID == "" {
		return errEmptyStoreID
	}

	categoryWrapper := platform.CategoryService{
		Client:           factory.NewPlatformClient(p.ConfigRepo),
		ConfigRepository: p.ConfigRepo,
		TokenRepository:  p.TokenRepo,
	}
	_, err := categoryWrapper.CreateCategoryShort(&category.CreateCategoryParams{
		Namespace: p.CLIConfig.ABNamespace,
		StoreID:   p.storeID,
		Body: &platformclientmodels.CategoryCreate{
			CategoryPath: &categoryPath,
			LocalizationDisplayNames: map[string]string{
				"en": categoryPath,
			},
		},
	})
	if err != nil {
		return fmt.Errorf("could not create new category: %w", err)
	}

	return nil
}

func (p *PlatformDataUnit) CreateCurrency() error {
	currencyWrapper := platform.CurrencyService{
		Client:           factory.NewPlatformClient(p.ConfigRepo),
		ConfigRepository: p.ConfigRepo,
		TokenRepository:  p.TokenRepo,
	}
	curr, err := currencyWrapper.GetCurrencySummaryShort(&currency.GetCurrencySummaryParams{
		CurrencyCode: p.CurrencyCode,
		Namespace:    p.CLIConfig.ABNamespace,
	})
	if err == nil && curr != nil {
		// already exists
		return nil
	}

	_, err = currencyWrapper.CreateCurrencyShort(&currency.CreateCurrencyParams{
		Namespace: p.CLIConfig.ABNamespace,
		Body: &platformclientmodels.CurrencyCreate{
			CurrencyCode:   Ptr(p.CurrencyCode),
			CurrencySymbol: "USDT1$",
			CurrencyType:   platformclientmodels.CurrencyCreateCurrencyTypeVIRTUAL,
			Decimals:       0,
		},
	})

	return err
}

func (p *PlatformDataUnit) DeleteCurrency() error {
	currencyWrapper := platform.CurrencyService{
		Client:           factory.NewPlatformClient(p.ConfigRepo),
		ConfigRepository: p.ConfigRepo,
		TokenRepository:  p.TokenRepo,
	}
	_, err := currencyWrapper.DeleteCurrencyShort(&currency.DeleteCurrencyParams{
		Namespace:    p.CLIConfig.ABNamespace,
		CurrencyCode: p.CurrencyCode,
	})

	return err
}

func (p *PlatformDataUnit) UnsetPlatformServiceGrpcTarget() error {
	servicePluginCfgWrapper := platform.ServicePluginConfigService{
		Client:           factory.NewPlatformClient(p.ConfigRepo),
		ConfigRepository: p.ConfigRepo,
		TokenRepository:  p.TokenRepo,
	}

	return servicePluginCfgWrapper.DeleteLootBoxPluginConfigShort(&service_plugin_config.DeleteLootBoxPluginConfigParams{
		Namespace: p.CLIConfig.ABNamespace,
	})
}

func (p *PlatformDataUnit) DeleteStore() error {
	storeWrapper := platform.StoreService{
		Client:           factory.NewPlatformClient(p.ConfigRepo),
		ConfigRepository: p.ConfigRepo,
		TokenRepository:  p.TokenRepo,
	}
	_, err := storeWrapper.DeleteStoreShort(&store.DeleteStoreParams{
		Namespace: p.CLIConfig.ABNamespace,
		StoreID:   p.storeID,
	})
	if err != nil {
		return err
	}

	_, err = storeWrapper.DeletePublishedStoreShort(&store.DeletePublishedStoreParams{
		Namespace: p.CLIConfig.ABNamespace,
	})

	return fmt.Errorf("delete published store error: %w", err)
}

func (p *PlatformDataUnit) CreateLootboxItems(itemCount int, rewardItemCount int, categoryPath string, doPublish bool) ([]SimpleLootboxItem, error) {
	if p.storeID == "" {
		return nil, errEmptyStoreID
	}

	itemWrapper := platform.ItemService{
		Client:           factory.NewPlatformClient(p.ConfigRepo),
		ConfigRepository: p.ConfigRepo,
		TokenRepository:  p.TokenRepo,
	}

	lootboxDiff := RandomString(ALPHA_CHARS, 6)
	var lootboxItems []SimpleLootboxItem

	for i := 0; i < itemCount; i++ {
		lootboxItem := SimpleLootboxItem{
			Title: fmt.Sprintf("Lootbox Item %s Titled %d", lootboxDiff, i+1),
			SKU:   fmt.Sprintf("SKUCL_%s_%d", lootboxDiff, i+1),
			Diff:  lootboxDiff,
		}

		var lootboxRewards []*platformclientmodels.LootBoxReward
		var rewardItems []SimpleItemInfo
		for j := 0; j < rewardItemCount; j++ {
			itemDiff := RandomString(ALPHA_CHARS, 6)
			items, err := p.CreateItems(1, categoryPath, itemDiff, doPublish)
			if err != nil {
				return nil, err
			}

			var rewardBoxItems []*platformclientmodels.BoxItem
			for _, itemInfo := range items {
				rewardBoxItems = append(rewardBoxItems, &platformclientmodels.BoxItem{
					Count:   1,
					ItemID:  itemInfo.ID,
					ItemSku: itemInfo.SKU,
				})
				rewardItems = append(rewardItems, itemInfo)
			}

			lootboxReward := platformclientmodels.LootBoxReward{
				Name:         fmt.Sprintf("Reward-%s", itemDiff),
				Odds:         0.1,
				Weight:       10,
				Type:         platformclientmodels.LootBoxRewardTypeREWARD,
				LootBoxItems: rewardBoxItems,
			}
			lootboxRewards = append(lootboxRewards, &lootboxReward)
		}

		lootboxItem.RewardItems = rewardItems

		newItem, err := itemWrapper.CreateItemShort(&item.CreateItemParams{
			Namespace: p.CLIConfig.ABNamespace,
			StoreID:   p.storeID,
			Body: &platformclientmodels.ItemCreate{
				Name:            Ptr(lootboxItem.Title),
				ItemType:        Ptr(platformclientmodels.ItemCreateItemTypeLOOTBOX),
				CategoryPath:    Ptr(categoryPath),
				EntitlementType: Ptr(platformclientmodels.ItemCreateEntitlementTypeCONSUMABLE),
				SeasonType:      platformclientmodels.ItemCreateSeasonTypeTIER,
				Status:          Ptr(platformclientmodels.ItemCreateStatusACTIVE),
				UseCount:        100,
				Listable:        true,
				Purchasable:     true,
				Sku:             lootboxItem.SKU,
				LootBoxConfig: &platformclientmodels.LootBoxConfig{
					RewardCount:  int32(rewardItemCount),
					Rewards:      lootboxRewards,
					RollFunction: platformclientmodels.LootBoxConfigRollFunctionCUSTOM,
				},
				Localizations: map[string]platformclientmodels.Localization{
					"en": {
						Title: Ptr(lootboxItem.Title),
					},
				},
				RegionData: map[string][]platformclientmodels.RegionDataItemDTO{
					"US": {
						{
							CurrencyCode:      Ptr(p.CurrencyCode),
							CurrencyNamespace: Ptr(p.CLIConfig.ABNamespace),
							CurrencyType:      Ptr(platformclientmodels.RegionDataItemDTOCurrencyTypeREAL),
							Price:             Ptr(int32((i + 1) * 2)),
						},
					},
				},
			},
		})
		if err != nil {
			return nil, err
		}

		if newItem == nil {
			return nil, fmt.Errorf("could not create new lootbox item")
		}
		lootboxItem.ID = *newItem.ItemID
		lootboxItems = append(lootboxItems, lootboxItem)
	}

	if doPublish {
		if err := p.publishStoreChange(); err != nil {
			return nil, err
		}
	}

	return lootboxItems, nil
}

func (p *PlatformDataUnit) CreateItems(itemCount int, categoryPath string, itemDiff string, doPublish bool) ([]SimpleItemInfo, error) {
	if p.storeID == "" {
		return nil, errEmptyStoreID
	}

	itemWrapper := platform.ItemService{
		Client:           factory.NewPlatformClient(p.ConfigRepo),
		ConfigRepository: p.ConfigRepo,
		TokenRepository:  p.TokenRepo,
	}
	var items []SimpleItemInfo
	for i := 0; i < itemCount; i++ {
		itemInfo := SimpleItemInfo{
			Title: fmt.Sprintf("Item %s Titled %d", itemDiff, i+1),
			SKU:   fmt.Sprintf("SKU_%s_%d", itemDiff, i+1),
		}

		newItem, err := itemWrapper.CreateItemShort(&item.CreateItemParams{
			Namespace: p.CLIConfig.ABNamespace,
			StoreID:   p.storeID,
			Body: &platformclientmodels.ItemCreate{
				Name:            &itemInfo.Title,
				ItemType:        Ptr(platformclientmodels.ItemCreateItemTypeSEASON),
				CategoryPath:    &categoryPath,
				EntitlementType: Ptr(platformclientmodels.ItemCreateEntitlementTypeDURABLE),
				SeasonType:      platformclientmodels.ItemCreateSeasonTypeTIER,
				Status:          Ptr(platformclientmodels.ItemCreateStatusACTIVE),
				Listable:        true,
				Purchasable:     true,
				Sku:             itemInfo.SKU,
				Localizations: map[string]platformclientmodels.Localization{
					"en": {
						Title: Ptr(itemInfo.Title),
					},
				},
				RegionData: map[string][]platformclientmodels.RegionDataItemDTO{
					"US": {
						{
							CurrencyCode:      Ptr(p.CurrencyCode),
							CurrencyNamespace: Ptr(p.CLIConfig.ABNamespace),
							CurrencyType:      Ptr(platformclientmodels.RegionDataItemDTOCurrencyTypeREAL),
							Price:             Ptr(int32((i + 1) * 2)),
						},
					},
				},
			},
		})
		if err != nil {
			return nil, fmt.Errorf("could not create new store item: %w", err)
		}

		itemInfo.ID = *newItem.ItemID
		items = append(items, itemInfo)
	}

	if doPublish {
		if err := p.publishStoreChange(); err != nil {
			return nil, err
		}
	}

	return items, nil
}

func (p *PlatformDataUnit) GrantEntitlement(userID string, itemID string, count int32) (string, error) {
	if p.storeID == "" {
		return "", errEmptyStoreID
	}

	entitlementWrapper := platform.EntitlementService{
		Client:           factory.NewPlatformClient(p.ConfigRepo),
		ConfigRepository: p.ConfigRepo,
		TokenRepository:  p.TokenRepo,
	}
	entitlementInfo, err := entitlementWrapper.GrantUserEntitlementShort(&entitlement.GrantUserEntitlementParams{
		Namespace: p.CLIConfig.ABNamespace,
		UserID:    userID,
		Body: []*platformclientmodels.EntitlementGrant{
			{
				ItemID:        Ptr(itemID),
				Quantity:      Ptr(count),
				Source:        platformclientmodels.EntitlementGrantSourceGIFT,
				StoreID:       p.storeID,
				ItemNamespace: Ptr(p.CLIConfig.ABNamespace),
			},
		},
	})
	if err != nil {
		return "", err
	}
	if len(entitlementInfo) <= 0 {
		return "", fmt.Errorf("could not grant item to user")
	}

	return Val(entitlementInfo[0].ID), nil
}

func (p *PlatformDataUnit) ConsumeItemEntitlement(userID string, entitlementID string, count int32) (*SimpleLootboxItem, error) {
	if p.storeID == "" {
		return nil, errEmptyStoreID
	}

	entitlementWrapper := platform.EntitlementService{
		Client:           factory.NewPlatformClient(p.ConfigRepo),
		ConfigRepository: p.ConfigRepo,
		TokenRepository:  p.TokenRepo,
	}
	result, err := entitlementWrapper.ConsumeUserEntitlementShort(&entitlement.ConsumeUserEntitlementParams{
		Namespace:     p.CLIConfig.ABNamespace,
		EntitlementID: entitlementID,
		UserID:        userID,
		Body: &platformclientmodels.AdminEntitlementDecrement{
			UseCount:  count,
			RequestID: RandomString(ALPHA_CHARS, 8),
		},
	})
	if err != nil {
		return nil, err
	}

	lootboxItem := SimpleLootboxItem{
		ID: Val(result.ID),
	}
	items := make([]SimpleItemInfo, len(result.Rewards))
	for _, it := range result.Rewards {
		items = append(items, SimpleItemInfo{
			ID:  it.ItemID,
			SKU: it.ItemSku,
		})
	}
	lootboxItem.RewardItems = items

	return &lootboxItem, nil
}

package backend

import (
	"fmt"

	v1 "kusionstack.io/kusion/pkg/apis/api.kusion.io/v1"
	"kusionstack.io/kusion/pkg/domain/constant"
	"kusionstack.io/kusion/pkg/domain/entity"
)

// MaskBackendSensitiveData is a helper function to mask sensitive data in backend
func MaskBackendSensitiveData(entity *entity.Backend) (*entity.Backend, error) {
	if entity == nil {
		return nil, ErrInternalServerError
	}

	// mask access secret key
	if _, ok := entity.BackendConfig.Configs[v1.BackendGenericOssSK]; ok {
		entity.BackendConfig.Configs[v1.BackendGenericOssSK] = "**********"
	}
	// mask access secret ID
	if _, ok := entity.BackendConfig.Configs[v1.BackendGenericOssAK]; ok {
		entity.BackendConfig.Configs[v1.BackendGenericOssAK] = "**********"
	}

	// mask google credentials
	if credentialsJSON, ok := entity.BackendConfig.Configs[v1.BackendGoogleCredentials].(map[string]any); ok {
		maskSensitiveData(credentialsJSON)
		entity.BackendConfig.Configs[v1.BackendGoogleCredentials] = credentialsJSON
	}
	return entity, nil
}

func maskSensitiveData(credentials map[string]any) {
	sensitiveFields := []string{"private_key", "client_email", "client_id"}
	for _, field := range sensitiveFields {
		if _, ok := credentials[field]; ok {
			credentials[field] = "**********"
		}
	}
}

func validateBackendSortOptions(sortBy string) (string, error) {
	if sortBy == "" {
		return constant.SortByID, nil
	}
	if sortBy != constant.SortByID && sortBy != constant.SortByName && sortBy != constant.SortByCreateTimestamp {
		return "", fmt.Errorf("invalid sort option: %s. Can only sort by id or create timestamp", sortBy)
	}
	switch sortBy {
	case constant.SortByCreateTimestamp:
		return "created_at", nil
	case constant.SortByModifiedTimestamp:
		return "updated_at", nil
	}
	return sortBy, nil
}

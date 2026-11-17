package models

import (
	"errors"
	"ragflow/internal/common"
)

// NewPrivateDeepSeekChat reuses the resolved native driver and complete opaque
// credential. The isolated client prevents redirects and source-payload logging.
func NewPrivateDeepSeekChat(driver ModelDriver, name *string, config *APIConfig, info *ModelInfo) (*ChatModel, error) {
	resolved, ok := driver.(*DeepSeekModel)
	if !ok || resolved == nil {
		return nil, errors.New("unsupported private provider")
	}
	copy := *resolved
	copy.baseModel.httpClient = common.PrivateProviderHTTPClient()
	return NewChatModelWithInfo(&copy, name, config, info), nil
}

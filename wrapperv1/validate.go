package wrapperv1

import "fmt"

func ValidateSearchResponse(resp SearchResponse) error {
	if resp.Items == nil {
		return fmt.Errorf("items is required")
	}
	for i, item := range resp.Items {
		if item.ID == "" {
			return fmt.Errorf("items[%d].id is required", i)
		}
	}
	return nil
}

func ValidateGetResponse(resp GetResponse) error {
	if resp.ID == "" {
		return fmt.Errorf("id is required")
	}
	return nil
}

func ValidateContentResponse(resp ContentResponse) error {
	if resp.ID == "" {
		return fmt.Errorf("id is required")
	}
	return nil
}

func ValidateErrorResponse(resp ErrorResponse) error {
	if resp.Error.Code == "" {
		return fmt.Errorf("error.code is required")
	}
	if resp.Error.Message == "" {
		return fmt.Errorf("error.message is required")
	}
	return nil
}

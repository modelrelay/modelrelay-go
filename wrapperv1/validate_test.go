package wrapperv1

import "testing"

func TestValidateSearchResponse(t *testing.T) {
	if err := ValidateSearchResponse(SearchResponse{}); err == nil {
		t.Fatal("expected error")
	}
	if err := ValidateSearchResponse(SearchResponse{Items: []Item{{ID: "x"}}}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateGetResponse(t *testing.T) {
	if err := ValidateGetResponse(GetResponse{}); err == nil {
		t.Fatal("expected error")
	}
}

func TestValidateContentResponse(t *testing.T) {
	if err := ValidateContentResponse(ContentResponse{}); err == nil {
		t.Fatal("expected error")
	}
}

func TestValidateErrorResponse(t *testing.T) {
	if err := ValidateErrorResponse(ErrorResponse{}); err == nil {
		t.Fatal("expected error")
	}
	if err := ValidateErrorResponse(ErrorResponse{Error: ErrorBody{Code: "x", Message: "oops"}}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

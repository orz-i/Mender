package application

import "errors"

var (
	ErrRuleNotFound              = errors.New("policy: rule not found")
	ErrRuleConflict              = errors.New("policy: rule already exists")
	ErrProfileReferenceNotFound  = errors.New("policy: Profile reference not found")
	ErrProviderReferenceNotFound = errors.New("policy: Provider reference not found")
	ErrToolReferenceNotFound     = errors.New("policy: Tool reference not found")
)

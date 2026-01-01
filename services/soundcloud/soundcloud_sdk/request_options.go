package soundcloud_sdk

import (
	"net/url"
)

type RequestOption func(*requestOptions)

type requestOptions struct {
	urlParams url.Values
}

type IncludeOption interface {
	~string
}

func includeOption[T IncludeOption](options ...T) RequestOption {
	return func(ro *requestOptions) {
		if len(options) == 0 {
			return
		}

		for _, opt := range options {
			ro.urlParams.Add("include", string(opt))
		}
	}
}

func buildRequestOptions(options ...RequestOption) requestOptions {
	op := requestOptions{
		urlParams: url.Values{},
	}

	for _, opt := range options {
		opt(&op)
	}
	return op
}

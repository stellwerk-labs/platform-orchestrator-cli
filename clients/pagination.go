package clients

import (
	"net/http"
	"reflect"

	"github.com/pkg/errors"

	"github.com/stellwerk-labs/platform-orchestrator-cli/internal/ref"
)

type PaginationResponse interface {
	StatusCode() int
}

func CollectAll[x any, r PaginationResponse](
	next func(string) (r, error),
	extract func(r) ([]x, *string),
) ([]x, error) {
	allItems := make([]x, 0)
	for pageToken, pageNum := "", 1; pageNum == 1 || pageToken != ""; pageNum++ {
		if res, err := next(pageToken); err != nil {
			return nil, errors.Wrap(err, "failed to list items")
		} else if res.StatusCode() != http.StatusOK {
			v := reflect.ValueOf(res)
			if v.Kind() == reflect.Pointer {
				v = v.Elem()
			}
			f := v.FieldByName("Body")
			if f.IsValid() {
				return nil, errors.Errorf("unexpected status code %d when listing items: %s", res.StatusCode(), string(f.Bytes()))
			}
			return nil, errors.Errorf("unexpected status code %d when listing items: %v", res.StatusCode(), res)
		} else {
			items, nextPageToken := extract(res)
			allItems = append(allItems, items...)
			pageToken = ref.DerefOr(nextPageToken, "")
		}
	}
	return allItems, nil
}

package imagecheck

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

var client = &http.Client{
	Timeout: 10 * time.Second,
}

func FilterURL(
	ctx context.Context,
	imageURL *string,
	maxMB int64,
) *string {
	if imageURL == nil {
		return nil
	}

	url := strings.TrimSpace(*imageURL)
	if url == "" {
		return nil
	}

	maxBytes := maxMB * 1024 * 1024

	size, err := getImageSize(ctx, url)
	if err != nil {
		fmt.Printf(
			"[IMAGE] Не удалось определить размер %s: %v. Картинку пропускаю\n",
			url,
			err,
		)

		return nil
	}

	if size > maxBytes {
		fmt.Printf(
			"[IMAGE] Картинка слишком большая: %.2f MB, максимум %d MB. Пропускаю\n",
			float64(size)/(1024*1024),
			maxMB,
		)

		return nil
	}

	return imageURL
}

func getImageSize(
	ctx context.Context,
	imageURL string,
) (int64, error) {
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodHead,
		imageURL,
		nil,
	)
	if err != nil {
		return 0, err
	}

	resp, err := client.Do(req)

	if err == nil {
		resp.Body.Close()

		if resp.StatusCode >= 200 &&
			resp.StatusCode < 400 &&
			resp.ContentLength >= 0 {

			return resp.ContentLength, nil
		}
	}

	req, err = http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		imageURL,
		nil,
	)
	if err != nil {
		return 0, err
	}

	req.Header.Set(
		"Range",
		"bytes=0-0",
	)

	resp, err = client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	contentRange := resp.Header.Get(
		"Content-Range",
	)

	if contentRange != "" {
		index := strings.LastIndex(
			contentRange,
			"/",
		)

		if index != -1 {
			total := contentRange[index+1:]

			if total != "*" {
				size, err := strconv.ParseInt(
					total,
					10,
					64,
				)

				if err == nil {
					return size, nil
				}
			}
		}
	}

	if resp.StatusCode == http.StatusOK &&
		resp.ContentLength >= 0 {

		return resp.ContentLength, nil
	}

	return 0, fmt.Errorf(
		"сервер не сообщил размер изображения",
	)
}

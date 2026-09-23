package model

import "time"

type Article struct {
	ID int64 `json:"id"`

	ExternalID string `json:"-"`

	TitleTM *string `json:"title_tm"`
	TitleRU *string `json:"title_ru"`
	TitleEN *string `json:"title_en"`

	TextTM *string `json:"text_tm"`
	TextRU *string `json:"text_ru"`
	TextEN *string `json:"text_en"`

	PostedAt *time.Time `json:"posted_at"`

	ScrapedAt time.Time `json:"scrapped_at"`

	ImgURL *string `json:"img_url"`

	URLTM *string `json:"url_tm"`
	URLRU *string `json:"url_ru"`
	URLEN *string `json:"url_en"`

	SourceName string `json:"source_name"`

	Published bool `json:"published"`

	Category   *string  `json:"-"`
	Categories []string `json:"categories"`
}

CREATE TABLE IF NOT EXISTS articles (
    id BIGSERIAL PRIMARY KEY,

    external_id VARCHAR(255) NOT NULL, 
    source_name VARCHAR(100) NOT NULL,

    title_tm TEXT,
    title_ru TEXT,
    title_en TEXT,

    text_tm TEXT,
    text_ru TEXT,
    text_en TEXT,

    posted_at TIMESTAMPTZ,
    scraped_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    img_url TEXT,
    url TEXT NOT NULL, 

    published BOOLEAN NOT NULL DEFAULT TRUE, 

    category VARCHAR(100),

    UNIQUE (source_name, external_id)


);

CREATE INDEX IF NOT EXISTS idx_articles_posted_at
ON articles (posted_at DESC);

CREATE INDEX IF NOT EXISTS idx_articles_category
ON articles (category);
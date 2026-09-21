
CREATE TABLE categories (
    id BIGSERIAL PRIMARY KEY,
    slug VARCHAR(100) NOT NULL UNIQUE
);

CREATE TABLE article_categories (
    article_id BIGINT NOT NULL
        REFERENCES articles(id)
        ON DELETE CASCADE,

    category_id BIGINT NOT NULL
        REFERENCES categories(id)
        ON DELETE CASCADE,

    PRIMARY KEY (article_id, category_id)
);

CREATE INDEX idx_article_categories_category_id
ON article_categories(category_id);


-- Переносим категории, которые уже успели накопиться
-- в старом articles.category.

INSERT INTO categories (slug)
SELECT DISTINCT category
FROM articles
WHERE category IS NOT NULL
  AND category <> ''
ON CONFLICT (slug) DO NOTHING;


INSERT INTO article_categories (
    article_id,
    category_id
)
SELECT
    a.id,
    c.id
FROM articles a
JOIN categories c
    ON c.slug = a.category
WHERE a.category IS NOT NULL
  AND a.category <> ''
ON CONFLICT DO NOTHING;
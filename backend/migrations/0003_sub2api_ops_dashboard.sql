-- Store the Sub2API admin connection separately from ordinary upstream channels.
CREATE TABLE IF NOT EXISTS sub2api_ops_configs (
    id               BIGINT PRIMARY KEY,
    name             VARCHAR(128) NOT NULL,
    site_url         VARCHAR(512) NOT NULL,
    admin_key_cipher TEXT NOT NULL,
    dashboard_layout TEXT NOT NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

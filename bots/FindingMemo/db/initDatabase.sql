SET TIME ZONE 'UTC'

CREATE TABLE IF NOT EXISTS users(
    user_id bigint NOT NULL PRIMARY KEY,
    chat_id bigint NOT NULL,
    remind boolean NOT NULL,
    remind_at smallint NOT NULL,
    latitude real NULL,
    longitude real NULL,
    timezone text NOT NULL
);

CREATE TABLE IF NOT EXISTS memos(
    memo_id int PRIMARY KEY GENERATED ALWAYS AS IDENTITY,
    chat_id bigint NOT NULL,
    text text NOT NULL,
    state smallint NOT NULL CHECK (state >= 0),
    priority smallint NOT NULL CHECK (priority > 0),
    timestamp timestamp NULL
);
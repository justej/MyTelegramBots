SET TIME ZONE 'UTC'

CREATE DATABASE alain_debot;

CREATE TABLE IF NOT EXISTS users (
    id bigint NOT NULL PRIMARY KEY,
    chat_id bigint NOT NULL UNIQUE,
    created_on timestamp NOT NULL
);

CREATE TABLE IF NOT EXISTS movies (
    id serial NOT NULL PRIMARY KEY,
    title text NOT NULL,
    alt_title text NULL,
    year smallint NULL,
    created_on timestamp NOT NULL,
    created_by bigint NOT NULL,
    updated_on timestamp NULL,
    deleted bool NULL,

    FOREIGN KEY (created_by) REFERENCES users (id)
);

CREATE INDEX movies_title_key ON movies USING btree (
    title ASC
);

CREATE INDEX movies_alt_title_key ON movies USING btree (
    alt_title ASC
);

CREATE TABLE IF NOT EXISTS ratings (
    user_id bigint NOT NULL,
    movie_id int NOT NULL,
    rating smallint NOT NULL,
    created_on timestamp NOT NULL,
    updated_on timestamp NULL,

    PRIMARY KEY (user_id, movie_id),
    FOREIGN KEY (user_id) REFERENCES users (id),
    FOREIGN KEY (movie_id) REFERENCES movies (id)
);

CREATE INDEX movies_movie_id_key ON ratings USING btree (
    movie_id ASC
);
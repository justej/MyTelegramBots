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

INSERT INTO users (user_id, chat_id, remind, remind_at)
VALUES
    (1, 1, TRUE, '9:00'),
    (2, 2, TRUE, '10:00+3'),
    (3, 3, false, '9:00');

INSERT INTO memos (memo_id, chat_id, text, state, priority, done_at)
VALUES
    (DEFAULT, 1, 'My memo', 0, 1, NULL),
    (DEFAULT, 2, 'Buy milk', 0, 1, NULL),
    (DEFAULT, 2, 'Read the paper', 0, 2, NULL),
    (DEFAULT, 3, 'Turn off light', 0, 1, NULL),
    (DEFAULT, 3, 'Turn on light', 0, 1, '9:10');

INSERT INTO memos(chat_id, text, state, priority)
VALUES
    (5,
     'Wash the car',
     0,
     COALESCE((SELECT max(priority) FROM memos WHERE chat_id = 5 AND state = 0), 0) + 1); 

INSERT INTO memos(chat_id, text, state, priority)
VALUES(1, 2, 3, COALESCE((SELECT max(priority) FROM memos WHERE chat_id=1 AND state=0), 0)+1)

SELECT text, state, done_at, priority FROM memos WHERE chat_id=1 and state<>2 ORDER BY priority ASC
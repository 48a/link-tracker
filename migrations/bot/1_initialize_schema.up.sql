create table if not exists user_states (
    chat_id bigint primary key,
    state int not null default 0,
    url text not null default '',
    tags text[] not null default '{}'
);

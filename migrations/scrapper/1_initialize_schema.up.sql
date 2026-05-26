create table if not exists chats (
	chat_id bigint primary key
);

create table if not exists links (
	id serial primary key,
	url text unique constraint url_length_check check (char_length(url) < 56692),
	last_updated timestamptz
);

create table if not exists chats_links (
	id serial primary key,
	chat_id bigint not null references chats(chat_id) on delete cascade,
	link_id integer not null references links(id) on delete cascade,
	unique (chat_id, link_id)
);

create table if not exists tags (
	tag text constraint tag_length_check check (char_length(tag) < 56692),
	sub_id integer references chats_links(id) on delete cascade,
	primary key (sub_id, tag)
);

-- +goose Up

-- Every string column compares bytes (COLLATE "C"), as Go does, so that
-- indexes return rows in the order that queries sort and page by, and
-- comparisons skip the locale.

CREATE TABLE "gebra_store" (
    "id" varchar(26) COLLATE "C",
    "name" varchar(256) COLLATE "C" NOT NULL,
    "model_id" varchar(26) COLLATE "C" NOT NULL,
    "revision" bigint NOT NULL,
    "created_at" timestamptz NOT NULL,
    "updated_at" timestamptz NOT NULL,
    "deleted_at" timestamptz,
    PRIMARY KEY ("id")
);

CREATE INDEX "gebra_store_deleted_at" ON "gebra_store" ("deleted_at");

CREATE TABLE "gebra_model" (
    "id" varchar(26) COLLATE "C",
    "store_id" varchar(26) COLLATE "C" NOT NULL,
    "definition" text NOT NULL,
    "created_at" timestamptz NOT NULL,
    PRIMARY KEY ("id")
);

CREATE INDEX "gebra_model_store" ON "gebra_model" ("store_id");

CREATE TABLE "gebra_assertion" (
    "store_id" varchar(26) COLLATE "C",
    "model_id" varchar(26) COLLATE "C",
    "definition" text NOT NULL,
    PRIMARY KEY ("store_id","model_id")
);

CREATE TABLE "gebra_tuple" (
    "id" bigserial,
    "store_id" varchar(26) COLLATE "C" NOT NULL,
    "object_type" varchar(64) COLLATE "C" NOT NULL,
    "object_id" varchar(191) COLLATE "C" NOT NULL,
    "relation" varchar(64) COLLATE "C" NOT NULL,
    "user_type" varchar(64) COLLATE "C" NOT NULL,
    "user_id" varchar(191) COLLATE "C" NOT NULL,
    "user_relation" varchar(64) COLLATE "C" NOT NULL,
    "condition_name" varchar(64) COLLATE "C" NOT NULL,
    "condition_context" text NOT NULL,
    "created_at" timestamptz NOT NULL,
    PRIMARY KEY ("id")
);

CREATE INDEX "gebra_tuple_user" ON "gebra_tuple" ("store_id","user_type","user_id","user_relation","object_type","relation");

CREATE UNIQUE INDEX "gebra_tuple_key" ON "gebra_tuple" ("store_id","object_type","object_id","relation","user_type","user_id","user_relation");

CREATE TABLE "gebra_change" (
    "id" bigserial,
    "store_id" varchar(26) COLLATE "C" NOT NULL,
    "operation" varchar(32) COLLATE "C" NOT NULL,
    "object_type" varchar(64) COLLATE "C" NOT NULL,
    "object_id" varchar(191) COLLATE "C" NOT NULL,
    "relation" varchar(64) COLLATE "C" NOT NULL,
    "user_type" varchar(64) COLLATE "C" NOT NULL,
    "user_id" varchar(191) COLLATE "C" NOT NULL,
    "user_relation" varchar(64) COLLATE "C" NOT NULL,
    "condition_name" varchar(64) COLLATE "C" NOT NULL,
    "condition_context" text NOT NULL,
    "created_at" timestamptz NOT NULL,
    PRIMARY KEY ("id")
);

CREATE INDEX "gebra_change_store_id" ON "gebra_change" ("store_id","id");

CREATE TABLE "gebra_object_attribute" (
    "store_id" varchar(26) COLLATE "C",
    "object_type" varchar(64) COLLATE "C",
    "name" varchar(64) COLLATE "C",
    "object_id" varchar(191) COLLATE "C",
    "value" varchar(191) COLLATE "C" NOT NULL,
    PRIMARY KEY ("store_id","object_type","name","object_id")
);

CREATE INDEX "gebra_object_attribute_sort" ON "gebra_object_attribute" ("store_id","object_type","name","value","object_id");

CREATE TABLE "gebra_inbox" (
    "id" bigserial,
    "store_id" varchar(26) COLLATE "C" NOT NULL,
    "kind" varchar(16) COLLATE "C" NOT NULL,
    "changes" text NOT NULL,
    "tuples" bigint NOT NULL,
    "created_at" timestamptz NOT NULL,
    PRIMARY KEY ("id")
);

CREATE INDEX "gebra_inbox_store_id" ON "gebra_inbox" ("store_id","id");

CREATE TABLE "gebra_pending" (
    "inbox_id" bigint,
    "seq" bigint,
    "store_id" varchar(26) COLLATE "C" NOT NULL,
    "object_type" varchar(64) COLLATE "C" NOT NULL,
    "object_id" varchar(191) COLLATE "C" NOT NULL,
    "relation" varchar(64) COLLATE "C" NOT NULL,
    "user_type" varchar(64) COLLATE "C" NOT NULL,
    "user_id" varchar(191) COLLATE "C" NOT NULL,
    "user_relation" varchar(64) COLLATE "C" NOT NULL,
    "deleted" boolean NOT NULL,
    "condition_name" varchar(64) COLLATE "C" NOT NULL,
    "condition_context" text NOT NULL,
    "created_at" timestamptz NOT NULL,
    PRIMARY KEY ("inbox_id","seq")
);

CREATE INDEX "gebra_pending_key" ON "gebra_pending" ("store_id","object_type","object_id","relation","user_type","user_id","user_relation","inbox_id");

CREATE TABLE "gebra_view_member" (
    "store_id" varchar(26) COLLATE "C",
    "gen" bigint,
    "set_key" varchar(321) COLLATE "C",
    "user_type" varchar(64) COLLATE "C",
    "user_id" varchar(191) COLLATE "C",
    "set_type" varchar(64) COLLATE "C" NOT NULL,
    "set_rel" varchar(64) COLLATE "C" NOT NULL,
    "set_id" varchar(191) COLLATE "C" NOT NULL,
    PRIMARY KEY ("store_id","gen","set_key","user_type","user_id")
);

CREATE INDEX "gebra_view_member_user_set" ON "gebra_view_member" ("store_id","gen","user_type","user_id","set_type","set_rel","set_id") INCLUDE ("set_key");

CREATE TABLE "gebra_view_edge" (
    "store_id" varchar(26) COLLATE "C",
    "gen" bigint,
    "child_key" varchar(321) COLLATE "C",
    "parent_key" varchar(321) COLLATE "C",
    "refs" bigint NOT NULL,
    PRIMARY KEY ("store_id","gen","child_key","parent_key")
);

CREATE TABLE "gebra_view_reach" (
    "store_id" varchar(26) COLLATE "C",
    "gen" bigint,
    "desc_key" varchar(321) COLLATE "C",
    "anc_type" varchar(64) COLLATE "C",
    "anc_rel" varchar(64) COLLATE "C",
    "anc_id" varchar(191) COLLATE "C",
    "desc_type" varchar(64) COLLATE "C" NOT NULL,
    "desc_rel" varchar(64) COLLATE "C" NOT NULL,
    "desc_id" varchar(191) COLLATE "C" NOT NULL,
    PRIMARY KEY ("store_id","gen","desc_key","anc_type","anc_rel","anc_id")
);

CREATE INDEX "gebra_view_reach_anc" ON "gebra_view_reach" ("store_id","gen","anc_type","anc_id","anc_rel","desc_type","desc_rel");

CREATE TABLE "gebra_view_state" (
    "store_id" varchar(26) COLLATE "C",
    "applied" bigint NOT NULL,
    "gen" bigint NOT NULL DEFAULT 0,
    "model_id" varchar(26) COLLATE "C" NOT NULL DEFAULT '',
    "member_rows" bigint NOT NULL DEFAULT 0,
    "edge_rows" bigint NOT NULL DEFAULT 0,
    "reach_rows" bigint NOT NULL DEFAULT 0,
    "last_gen" bigint NOT NULL DEFAULT 0,
    "build_gen" bigint NOT NULL DEFAULT 0,
    "build_model_id" varchar(26) COLLATE "C" NOT NULL DEFAULT '',
    "build_phase" varchar(16) COLLATE "C" NOT NULL DEFAULT '',
    "build_object_type" varchar(64) COLLATE "C" NOT NULL DEFAULT '',
    "build_object_id" varchar(191) COLLATE "C" NOT NULL DEFAULT '',
    "build_relation" varchar(64) COLLATE "C" NOT NULL DEFAULT '',
    "build_user_type" varchar(64) COLLATE "C" NOT NULL DEFAULT '',
    "build_user_id" varchar(191) COLLATE "C" NOT NULL DEFAULT '',
    "build_user_relation" varchar(64) COLLATE "C" NOT NULL DEFAULT '',
    "build_set_key" varchar(321) COLLATE "C" NOT NULL DEFAULT '',
    "build_done" bigint NOT NULL DEFAULT 0,
    "build_total" bigint NOT NULL DEFAULT 0,
    "build_chunks" bigint NOT NULL DEFAULT 0,
    "build_member_rows" bigint NOT NULL DEFAULT 0,
    "build_edge_rows" bigint NOT NULL DEFAULT 0,
    "build_reach_rows" bigint NOT NULL DEFAULT 0,
    "build_started_at" timestamptz,
    "updated_at" timestamptz NOT NULL,
    PRIMARY KEY ("store_id")
);

CREATE TABLE "gebra_view_gc" (
    "store_id" varchar(26) COLLATE "C",
    "gen" bigint,
    "delete_after" timestamptz NOT NULL,
    PRIMARY KEY ("store_id","gen")
);

CREATE TABLE "gebra_change_cutoff" (
    "store_id" varchar(26) COLLATE "C",
    "change_id" bigint NOT NULL,
    "updated_at" timestamptz NOT NULL,
    PRIMARY KEY ("store_id")
);

CREATE TABLE "gebra_model_effect" (
    "model_id" varchar(26) COLLATE "C",
    "set_type" varchar(64) COLLATE "C",
    "set_rel" varchar(64) COLLATE "C",
    "tuple_rel" varchar(64) COLLATE "C",
    "kind" varchar(8) COLLATE "C",
    PRIMARY KEY ("model_id","set_type","set_rel","tuple_rel","kind")
);

-- A store's view rows are mostly of one generation, but without these
-- statistics Postgres assumes that store and generation are independent,
-- estimates a generation of a store at a fraction of its rows, and then
-- prefers scanning all of them, filtered by a list of keys, to probing the
-- index for each key.

CREATE STATISTICS gebra_view_member_store_gen (dependencies) ON store_id, gen FROM gebra_view_member;

CREATE STATISTICS gebra_view_edge_store_gen (dependencies) ON store_id, gen FROM gebra_view_edge;

CREATE STATISTICS gebra_view_reach_store_gen (dependencies) ON store_id, gen FROM gebra_view_reach;

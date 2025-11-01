package pgdump

import (
	"bytes"
	"io"
	"os"
	"runtime"
	"testing"

	"github.com/hexops/autogold"
	"github.com/stretchr/testify/require"
)

// createTestFile creates a temporary file with the given content for testing
func createTestFile(t *testing.T, content string) *os.File {
	src, err := os.CreateTemp(t.TempDir(), "test-*.sql")
	require.NoError(t, err)
	_, err = src.WriteString(content)
	require.NoError(t, err)
	_, err = src.Seek(0, io.SeekStart)
	require.NoError(t, err)
	return src
}

func TestCommentOutInvalidLines(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Test doesn't work on Windows of weirdness with t.TempDir() handling")
	}

	tests := []struct {
		name  string
		input string
		want  string
	}{

		{
			name: "EOF - input file doesn't contain any filterEndMarkers, or linesToFilter",
			input: `
--
-- PostgreSQL database dump
--
`,
			want: `
--
-- PostgreSQL database dump
--
`,
		},
		{
			name: "EOF - input file doesn't contain any filterEndMarkers, but does contain linesToFilter",
			input: `
DROP DATABASE pgsql;
`,
			want: `
-- DROP DATABASE pgsql;
`,
		},
		{
			name: "Customer-realistic dump, with extensions and schemas",
			input: `
--
-- PostgreSQL database dump
--

\restrict 1e9XN4yltwkS6RMoyhkFC6hmzrkbz4fZIVvJSYP3h5B1Qvii1WnhlslcPAzK8Tb

-- Dumped from database version 12.22
-- Dumped by pg_dump version 14.19 (Homebrew)

-- Started on 2025-10-29 20:49:56 IST

SET transaction_timeout = 10;
SET statement_timeout = 0;
SET lock_timeout = 0;
SET idle_in_transaction_session_timeout = 0;
SET client_encoding = 'UTF8';
SET standard_conforming_strings = on;
SELECT pg_catalog.set_config('search_path', '', false);
SET check_function_bodies = false;
SET xmloption = content;
SET client_min_messages = warning;
SET row_security = off;

DROP DATABASE pgsql IF EXISTS;
CREATE DATABASE pgsql;
COMMENT ON DATABASE pgsql IS 'database';

\connect pgsql

DROP SCHEMA public IF EXISTS;
CREATE SCHEMA public;
COMMENT ON SCHEMA public IS 'schema';

DROP EXTENSION IF EXISTS pg_stat_statements;
CREATE EXTENSION pg_stat_statements;
COMMENT ON EXTENSION pg_stat_statements IS 'extension';

ALTER TABLE IF EXISTS ONLY "public"."webhooks" DROP CONSTRAINT IF EXISTS "webhooks_updated_by_user_id_fkey";
DROP TRIGGER IF EXISTS "versions_insert" ON "public"."versions";
DROP INDEX IF EXISTS "public"."webhook_logs_status_code_idx";
DROP SEQUENCE IF EXISTS "public"."webhooks_id_seq";
DROP TABLE IF EXISTS "public"."webhooks";
SET default_tablespace = '';
SET default_table_access_method = "heap";

CREATE TABLE "public"."access_tokens" (
    "id" bigint NOT NULL,
    "subject_user_id" integer NOT NULL,
    "value_sha256" "bytea" NOT NULL,
    "note" "text" NOT NULL,
    "created_at" timestamp with time zone DEFAULT "now"() NOT NULL,
    "last_used_at" timestamp with time zone,
    "deleted_at" timestamp with time zone,
    "creator_user_id" integer NOT NULL,
    "scopes" "text"[] NOT NULL,
    "internal" boolean DEFAULT false
);

\unrestrict 1e9XN4yltwkS6RMoyhkFC6hmzrkbz4fZIVvJSYP3h5B1Qvii1WnhlslcPAzK8Tb
`,
			want: `
--
-- PostgreSQL database dump
--

\restrict 1e9XN4yltwkS6RMoyhkFC6hmzrkbz4fZIVvJSYP3h5B1Qvii1WnhlslcPAzK8Tb

-- Dumped from database version 12.22
-- Dumped by pg_dump version 14.19 (Homebrew)

-- Started on 2025-10-29 20:49:56 IST

-- SET transaction_timeout = 10;
SET statement_timeout = 0;
SET lock_timeout = 0;
SET idle_in_transaction_session_timeout = 0;
SET client_encoding = 'UTF8';
SET standard_conforming_strings = on;
SELECT pg_catalog.set_config('search_path', '', false);
SET check_function_bodies = false;
SET xmloption = content;
SET client_min_messages = warning;
SET row_security = off;

-- DROP DATABASE pgsql IF EXISTS;
-- CREATE DATABASE pgsql;
-- COMMENT ON DATABASE pgsql IS 'database';

-- \connect pgsql

-- DROP SCHEMA public IF EXISTS;
-- CREATE SCHEMA public;
-- COMMENT ON SCHEMA public IS 'schema';

-- DROP EXTENSION IF EXISTS pg_stat_statements;
-- CREATE EXTENSION pg_stat_statements;
-- COMMENT ON EXTENSION pg_stat_statements IS 'extension';

ALTER TABLE IF EXISTS ONLY "public"."webhooks" DROP CONSTRAINT IF EXISTS "webhooks_updated_by_user_id_fkey";
DROP TRIGGER IF EXISTS "versions_insert" ON "public"."versions";
DROP INDEX IF EXISTS "public"."webhook_logs_status_code_idx";
DROP SEQUENCE IF EXISTS "public"."webhooks_id_seq";
DROP TABLE IF EXISTS "public"."webhooks";
SET default_tablespace = '';
SET default_table_access_method = "heap";

CREATE TABLE "public"."access_tokens" (
    "id" bigint NOT NULL,
    "subject_user_id" integer NOT NULL,
    "value_sha256" "bytea" NOT NULL,
    "note" "text" NOT NULL,
    "created_at" timestamp with time zone DEFAULT "now"() NOT NULL,
    "last_used_at" timestamp with time zone,
    "deleted_at" timestamp with time zone,
    "creator_user_id" integer NOT NULL,
    "scopes" "text"[] NOT NULL,
    "internal" boolean DEFAULT false
);

\unrestrict 1e9XN4yltwkS6RMoyhkFC6hmzrkbz4fZIVvJSYP3h5B1Qvii1WnhlslcPAzK8Tb
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := createTestFile(t, tt.input)
			var dst bytes.Buffer

			_, err := CommentOutInvalidLines(&dst, src, func(i int64) {})
			require.NoError(t, err)

			// Copy rest of contents
			_, err = io.Copy(&dst, src)
			require.NoError(t, err)

			autogold.Want(tt.name, tt.want).Equal(t, dst.String())
		})
	}
}

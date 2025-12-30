#!/usr/bin/env python3
"""
Migration script to convert WakaTime data from MySQL to SQLite.

This script connects to the old MySQL database, reads all the data,
and inserts it into the new SQLite database format.

Usage:
    python3 migrate.py --mysql-host localhost --mysql-port 3306 \
        --mysql-user wakatime --mysql-password 123456 \
        --mysql-db wakatime --sqlite-path wakatime.db
"""

import argparse
import sqlite3
from datetime import datetime, date
from typing import Optional
import json

# Disable deprecated datetime adapters for Python 3.12+
sqlite3.register_adapter(datetime, lambda dt: dt.isoformat())
sqlite3.register_adapter(date, lambda d: d.isoformat())

try:
    import mysql.connector
except ImportError:
    print("Please install mysql-connector-python: pip3 install mysql-connector-python")
    exit(1)


current = sqlite3.sqlite_version.split(".")
required = "3.45.0".split(".")

for c, r in zip(current, required):
    if int(c) < int(r):
        print("Python update is required for JSONB features in SQLite 3.45.0+")
        exit(1)


def parse_args():
    parser = argparse.ArgumentParser(
        description="Migrate WakaTime data from MySQL to SQLite"
    )
    parser.add_argument("--mysql-host", default="localhost", help="MySQL host")
    parser.add_argument("--mysql-port", type=int, default=3306, help="MySQL port")
    parser.add_argument("--mysql-user", default="wakatime", help="MySQL username")
    parser.add_argument("--mysql-password", default="123456", help="MySQL password")
    parser.add_argument("--mysql-db", default="wakatime", help="MySQL database name")
    parser.add_argument(
        "--sqlite-path", default="wakatime.db", help="SQLite database path"
    )
    return parser.parse_args()


def create_sqlite_schema(sqlite_conn: sqlite3.Connection):
    """Create the SQLite schema"""
    cursor = sqlite_conn.cursor()

    # Projects table
    cursor.execute(
        """
        CREATE TABLE IF NOT EXISTS projects (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            uuid TEXT UNIQUE,
            name TEXT NOT NULL,
            repository TEXT,
            badge TEXT,
            color TEXT,
            has_public_url INTEGER DEFAULT 0,
            last_heartbeat_at DATETIME,
            first_heartbeat_at DATETIME,
            created_at DATETIME DEFAULT CURRENT_TIMESTAMP
        )
    """
    )

    # Durations table
    cursor.execute(
        """
        CREATE TABLE IF NOT EXISTS durations (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            day DATE NOT NULL,
            project TEXT,
            start_time REAL NOT NULL,
            duration REAL NOT NULL,
            dependencies JSONB,
            created_at DATETIME DEFAULT CURRENT_TIMESTAMP
        )
    """
    )
    cursor.execute("CREATE INDEX IF NOT EXISTS idx_durations_day ON durations(day)")

    # Project durations table
    cursor.execute(
        """
        CREATE TABLE IF NOT EXISTS project_durations (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            day DATE NOT NULL,
            project TEXT,
            branch TEXT,
            entity TEXT,
            language TEXT,
            type TEXT,
            start_time REAL NOT NULL,
            duration REAL NOT NULL,
            dependencies JSONB,
            created_at DATETIME DEFAULT CURRENT_TIMESTAMP
        )
    """
    )
    cursor.execute(
        "CREATE INDEX IF NOT EXISTS idx_project_durations_day ON project_durations(day)"
    )

    # Heartbeats table
    cursor.execute(
        """
        CREATE TABLE IF NOT EXISTS heartbeats (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            day DATE NOT NULL,
            entity TEXT NOT NULL,
            type TEXT,
            category TEXT,
            time REAL NOT NULL,
            project TEXT,
            branch TEXT,
            language TEXT,
            is_write INTEGER DEFAULT 0,
            machine_id TEXT,
            lines INTEGER,
            line_no INTEGER,
            cursor_pos INTEGER,
            created_at DATETIME DEFAULT CURRENT_TIMESTAMP
        )
    """
    )
    cursor.execute("CREATE INDEX IF NOT EXISTS idx_heartbeats_day ON heartbeats(day)")

    # Day summaries table
    cursor.execute(
        """
        CREATE TABLE IF NOT EXISTS day_summaries (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            day DATE NOT NULL UNIQUE,
            total_seconds REAL NOT NULL,
            created_at DATETIME DEFAULT CURRENT_TIMESTAMP
        )
    """
    )

    # Day stats table
    cursor.execute(
        """
        CREATE TABLE IF NOT EXISTS day_stats (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            day DATE NOT NULL,
            type TEXT NOT NULL,
            name TEXT NOT NULL,
            total_seconds REAL NOT NULL,
            created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
            UNIQUE(day, type, name)
        )
    """
    )
    cursor.execute("CREATE INDEX IF NOT EXISTS idx_day_stats_day ON day_stats(day)")

    # Sync log table
    cursor.execute(
        """
        CREATE TABLE IF NOT EXISTS sync_log (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            day DATE NOT NULL UNIQUE,
            synced_at DATETIME DEFAULT CURRENT_TIMESTAMP,
            total_seconds REAL,
            status TEXT DEFAULT 'success'
        )
    """
    )

    sqlite_conn.commit()


def datetime_to_timestamp(dt: Optional[datetime]) -> Optional[float]:
    """Convert datetime to Unix timestamp"""
    if dt is None:
        return None
    return dt.timestamp()


def date_to_str(d: Optional[date]) -> Optional[str]:
    """Convert date to string"""
    if d is None:
        return None
    return d.strftime("%Y-%m-%d")


def migrate_projects(mysql_conn, sqlite_conn: sqlite3.Connection):
    """Migrate projects table"""
    print("Migrating projects...")
    mysql_cursor = mysql_conn.cursor(dictionary=True)
    mysql_cursor.execute("SELECT * FROM project")

    sqlite_cursor = sqlite_conn.cursor()
    count = 0

    for row in mysql_cursor:
        sqlite_cursor.execute(
            """
            INSERT OR REPLACE INTO projects 
            (uuid, name, repository, badge, has_public_url, created_at)
            VALUES (?, ?, ?, ?, ?, ?)
        """,
            (
                row.get("uuid"),
                row.get("name"),
                row.get("repository"),
                None,  # badge not in old schema
                1 if row.get("public_url") else 0,
                row.get("created_time"),
            ),
        )
        count += 1

    sqlite_conn.commit()
    print(f"  Migrated {count} projects")


def migrate_durations(mysql_conn, sqlite_conn: sqlite3.Connection):
    """Migrate durations table"""
    print("Migrating durations...")
    mysql_cursor = mysql_conn.cursor(dictionary=True)
    mysql_cursor.execute("SELECT * FROM duration")

    sqlite_cursor = sqlite_conn.cursor()
    count = 0

    for row in mysql_cursor:
        start_time = row.get("start_time")
        if start_time:
            # Convert datetime to Unix timestamp
            timestamp = start_time.timestamp()
            day = start_time.strftime("%Y-%m-%d")

            # Handle dependencies - ensure it's valid JSON array format
            deps = row.get("dependencies")
            if deps:
                # If it's already a list/dict, convert to JSON string
                if isinstance(deps, (list, dict)):
                    deps = json.dumps(deps)
                # If it's a string, try to parse and re-encode to ensure valid JSON
                elif isinstance(deps, str):
                    try:
                        # Try to parse as JSON to validate/normalize
                        parsed = json.loads(deps)
                        deps = json.dumps(parsed)
                    except (json.JSONDecodeError, ValueError):
                        # If not valid JSON, treat as single item and create array
                        deps = json.dumps([deps]) if deps else json.dumps([])
            else:
                # Empty dependencies should be an empty JSON array
                deps = json.dumps([])

            sqlite_cursor.execute(
                """
                INSERT INTO durations 
                (day, project, start_time, duration, dependencies, created_at)
                VALUES (?, ?, ?, ?, jsonb(?), ?)
            """,
                (
                    day,
                    row.get("project"),
                    timestamp,
                    row.get("duration", 0),
                    deps,
                    row.get("created_time"),
                ),
            )
            count += 1

    sqlite_conn.commit()
    print(f"  Migrated {count} durations")


def migrate_project_durations(mysql_conn, sqlite_conn: sqlite3.Connection):
    """Migrate project_duration table"""
    print("Migrating project durations...")
    mysql_cursor = mysql_conn.cursor(dictionary=True)
    mysql_cursor.execute("SELECT * FROM project_duration")

    sqlite_cursor = sqlite_conn.cursor()
    count = 0

    for row in mysql_cursor:
        start_time = row.get("start_time")
        if start_time:
            timestamp = start_time.timestamp()
            day = start_time.strftime("%Y-%m-%d")

            # Handle dependencies - ensure it's valid JSON array format
            deps = row.get("dependencies")
            if deps:
                # If it's already a list/dict, convert to JSON string
                if isinstance(deps, (list, dict)):
                    deps = json.dumps(deps)
                # If it's a string, try to parse and re-encode to ensure valid JSON
                elif isinstance(deps, str):
                    try:
                        # Try to parse as JSON to validate/normalize
                        parsed = json.loads(deps)
                        deps = json.dumps(parsed)
                    except (json.JSONDecodeError, ValueError):
                        # If not valid JSON, treat as single item and create array
                        deps = json.dumps([deps]) if deps else json.dumps([])
            else:
                # Empty dependencies should be an empty JSON array
                deps = json.dumps([])

            sqlite_cursor.execute(
                """
                INSERT INTO project_durations 
                (day, project, branch, entity, language, type, start_time, duration, dependencies, created_at)
                VALUES (?, ?, ?, ?, ?, ?, ?, ?, jsonb(?), ?)
            """,
                (
                    day,
                    row.get("project"),
                    row.get("branch"),
                    row.get("entity"),
                    row.get("language"),
                    row.get("type"),
                    timestamp,
                    row.get("duration", 0),
                    deps,
                    row.get("created_time"),
                ),
            )
            count += 1

    sqlite_conn.commit()
    print(f"  Migrated {count} project durations")


def migrate_heartbeats(mysql_conn, sqlite_conn: sqlite3.Connection):
    """Migrate heart_beat table"""
    print("Migrating heartbeats...")
    mysql_cursor = mysql_conn.cursor(dictionary=True)
    mysql_cursor.execute("SELECT * FROM heart_beat")

    sqlite_cursor = sqlite_conn.cursor()
    count = 0

    for row in mysql_cursor:
        hb_time = row.get("time")
        if hb_time:
            timestamp = hb_time.timestamp()
            day = hb_time.strftime("%Y-%m-%d")

            sqlite_cursor.execute(
                """
                INSERT INTO heartbeats 
                (day, entity, type, time, created_at)
                VALUES (?, ?, ?, ?, ?)
            """,
                (
                    day,
                    row.get("name"),  # 'name' in old schema is 'entity'
                    row.get("type"),
                    timestamp,
                    row.get("created_time"),
                ),
            )
            count += 1

    sqlite_conn.commit()
    print(f"  Migrated {count} heartbeats")


def migrate_day_stats(mysql_conn, sqlite_conn: sqlite3.Connection):
    """Migrate day_* tables to unified day_stats table"""
    print("Migrating day stats...")

    tables_mapping = [
        ("day_category", "category"),
        ("day_language", "language"),
        ("day_editor", "editor"),
        ("day_operating_system", "os"),
        ("day_project", "project"),
        ("day_dependency", "dependency"),
    ]

    mysql_cursor = mysql_conn.cursor(dictionary=True)
    sqlite_cursor = sqlite_conn.cursor()
    total_count = 0

    for table, stat_type in tables_mapping:
        try:
            mysql_cursor.execute(f"SELECT * FROM {table}")
            count = 0

            for row in mysql_cursor:
                day = row.get("day")
                if day:
                    day_str = (
                        day.strftime("%Y-%m-%d") if isinstance(day, date) else str(day)
                    )

                    sqlite_cursor.execute(
                        """
                        INSERT OR REPLACE INTO day_stats 
                        (day, type, name, total_seconds, created_at)
                        VALUES (?, ?, ?, ?, ?)
                    """,
                        (
                            day_str,
                            stat_type,
                            row.get("name", ""),
                            row.get("total_seconds", 0),
                            row.get("created_time"),
                        ),
                    )
                    count += 1

            print(f"  Migrated {count} {stat_type} stats")
            total_count += count
        except Exception as e:
            print(f"  Warning: Could not migrate {table}: {e}")

    sqlite_conn.commit()
    print(f"  Total stats migrated: {total_count}")


def migrate_day_summaries(mysql_conn, sqlite_conn: sqlite3.Connection):
    """Migrate day_grand_total to day_summaries"""
    print("Migrating day summaries...")
    mysql_cursor = mysql_conn.cursor(dictionary=True)
    mysql_cursor.execute("SELECT * FROM day_grand_total")

    sqlite_cursor = sqlite_conn.cursor()
    count = 0

    for row in mysql_cursor:
        day = row.get("day")
        if day:
            day_str = day.strftime("%Y-%m-%d") if isinstance(day, date) else str(day)

            sqlite_cursor.execute(
                """
                INSERT OR REPLACE INTO day_summaries 
                (day, total_seconds, created_at)
                VALUES (?, ?, ?)
            """,
                (day_str, row.get("total_seconds", 0), row.get("created_time")),
            )
            count += 1

    sqlite_conn.commit()
    print(f"  Migrated {count} day summaries")


def create_sync_log_from_summaries(sqlite_conn: sqlite3.Connection):
    """Create sync_log entries from existing day_summaries"""
    print("Creating sync log entries...")
    cursor = sqlite_conn.cursor()

    cursor.execute(
        """
        INSERT OR IGNORE INTO sync_log (day, total_seconds, status)
        SELECT day, total_seconds, 'success' FROM day_summaries
    """
    )

    sqlite_conn.commit()
    print(f"  Created {cursor.rowcount} sync log entries")


def main():
    args = parse_args()

    print(f"Connecting to MySQL at {args.mysql_host}:{args.mysql_port}...")
    try:
        mysql_conn = mysql.connector.connect(
            host=args.mysql_host,
            port=args.mysql_port,
            user=args.mysql_user,
            password=args.mysql_password,
            database=args.mysql_db,
        )
    except Exception as e:
        print(f"Failed to connect to MySQL: {e}")
        return

    print(f"Creating SQLite database at {args.sqlite_path}...")
    sqlite_conn = sqlite3.connect(args.sqlite_path)

    try:
        # Create schema
        create_sqlite_schema(sqlite_conn)

        # Migrate data
        migrate_projects(mysql_conn, sqlite_conn)
        migrate_durations(mysql_conn, sqlite_conn)
        migrate_project_durations(mysql_conn, sqlite_conn)
        migrate_heartbeats(mysql_conn, sqlite_conn)
        migrate_day_stats(mysql_conn, sqlite_conn)
        migrate_day_summaries(mysql_conn, sqlite_conn)
        create_sync_log_from_summaries(sqlite_conn)

        print("\nMigration completed successfully!")
        print(f"SQLite database created at: {args.sqlite_path}")

    except Exception as e:
        print(f"Migration failed: {e}")
        import traceback

        traceback.print_exc()
    finally:
        mysql_conn.close()
        sqlite_conn.close()


if __name__ == "__main__":
    main()

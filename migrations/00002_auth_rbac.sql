CREATE TABLE aowugong_fastapi_users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username TEXT NOT NULL UNIQUE,
    email TEXT NOT NULL UNIQUE,
    password TEXT NOT NULL,
    full_name TEXT,
    phone TEXT,
    is_active INTEGER NOT NULL DEFAULT 1,
    is_superuser INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_aowugong_fastapi_users_username ON aowugong_fastapi_users (username);
CREATE INDEX idx_aowugong_fastapi_users_email ON aowugong_fastapi_users (email);

CREATE TABLE aowugong_roles (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    code TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT '',
    is_active INTEGER NOT NULL DEFAULT 1,
    is_system INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_aowugong_roles_code ON aowugong_roles (code);

CREATE TABLE aowugong_permissions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    code TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    "group" TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_aowugong_permissions_code ON aowugong_permissions (code);

CREATE TABLE aowugong_user_roles (
    user_id INTEGER NOT NULL,
    role_id INTEGER NOT NULL,
    PRIMARY KEY (user_id, role_id),
    FOREIGN KEY (user_id) REFERENCES aowugong_fastapi_users (id) ON DELETE CASCADE,
    FOREIGN KEY (role_id) REFERENCES aowugong_roles (id) ON DELETE CASCADE
);

CREATE INDEX idx_aowugong_user_roles_role_id ON aowugong_user_roles (role_id);

CREATE TABLE aowugong_role_permissions (
    role_id INTEGER NOT NULL,
    permission_id INTEGER NOT NULL,
    PRIMARY KEY (role_id, permission_id),
    FOREIGN KEY (role_id) REFERENCES aowugong_roles (id) ON DELETE CASCADE,
    FOREIGN KEY (permission_id) REFERENCES aowugong_permissions (id) ON DELETE CASCADE
);

CREATE INDEX idx_aowugong_role_permissions_permission_id ON aowugong_role_permissions (permission_id);

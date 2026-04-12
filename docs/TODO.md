# Known Issues for v1.0 Launch

- **In-memory session store (ATLA-195)**: The current session store is in-memory only. All users will lose their authenticated sessions and need to re-login if the proxy restarts. This is acceptable for v1.0. Redis/SQLite persistent session storage is planned for v1.1.

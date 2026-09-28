-- The single PostgreSQL container hosts two logical databases.
-- Kratos uses the `ory` database and the `ory` role created by the image.
-- Keto uses its own database and role so its migrations remain isolated.
CREATE USER keto WITH PASSWORD 'keto';
CREATE DATABASE keto OWNER keto;

-- Token Manager 的元数据与 Ory 数据库隔离，但仍复用同一个 PostgreSQL 实例。
CREATE USER token_manager WITH PASSWORD 'token_manager';
CREATE DATABASE token_manager OWNER token_manager;

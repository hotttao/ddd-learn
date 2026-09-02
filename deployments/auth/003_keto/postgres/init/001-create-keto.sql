-- The single PostgreSQL container hosts two logical databases.
-- Kratos uses the `ory` database and the `ory` role created by the image.
-- Keto uses its own database and role so its migrations remain isolated.
CREATE USER keto WITH PASSWORD 'keto';
CREATE DATABASE keto OWNER keto;

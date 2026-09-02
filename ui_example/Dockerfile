# Build stage: use the project's domestic Docker mirror and the domestic Yarn registry.
FROM docker.m.daocloud.io/library/node:22-bookworm AS build

WORKDIR /app

# Corepack provides the Yarn version declared by the project.
RUN corepack enable \
    && yarn config set registry https://registry.npmmirror.com

COPY package.json yarn.lock ./
RUN yarn install --frozen-lockfile --registry=https://registry.npmmirror.com

COPY . ./
RUN yarn build

# Runtime stage: Nginx only serves the compiled Vite assets.
FROM m.daocloud.io/docker.io/library/nginx:1.27-alpine

COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /app/dist/ /usr/share/nginx/html/

EXPOSE 80

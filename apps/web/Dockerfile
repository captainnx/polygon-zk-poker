FROM node:24-alpine AS build
WORKDIR /src
COPY package*.json ./
COPY apps/web/package.json apps/web/package.json
COPY apps/prover/package.json apps/prover/package.json
COPY packages/protocol/package.json packages/protocol/package.json
COPY packages/sdk/package.json packages/sdk/package.json
RUN npm ci --ignore-scripts
COPY tsconfig.json ./
COPY apps/web apps/web
COPY packages packages
RUN npm run build --workspace @zkpoker/web

FROM nginx:1.28-alpine
COPY apps/web/nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /src/apps/web/dist /usr/share/nginx/html
EXPOSE 80

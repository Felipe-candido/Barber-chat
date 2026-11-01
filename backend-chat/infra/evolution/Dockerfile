FROM node:24-alpine@sha256:8a32c8cf649a49915a66e81e7a609223c708d0adabf288ee3f4077d765542f85 AS build
RUN apk add --no-cache git ffmpeg bash openssl python3 make g++
WORKDIR /evolution
COPY --from=evolution_source / /evolution/
ENV DATABASE_PROVIDER=postgresql
ENV DATABASE_CONNECTION_URI=postgresql://build:build@127.0.0.1:5432/build
RUN npm ci --no-audit --no-fund
COPY compatibility.cjs /tmp/barberchat-evolution-compatibility.cjs
RUN node /tmp/barberchat-evolution-compatibility.cjs
RUN npm run db:generate && npm run build

FROM node:24-alpine@sha256:8a32c8cf649a49915a66e81e7a609223c708d0adabf288ee3f4077d765542f85
RUN apk add --no-cache ffmpeg bash openssl tzdata
WORKDIR /evolution
COPY --from=build /evolution /evolution
ENV NODE_ENV=production DOCKER_ENV=true DATABASE_PROVIDER=postgresql TELEMETRY_ENABLED=false
# No credentials are baked into the image or printed by the upstream deploy wrapper.
CMD ["/bin/sh", "-c", "npm run db:deploy && npm run start:prod"]

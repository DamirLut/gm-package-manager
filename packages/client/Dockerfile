FROM oven/bun:1.4 AS build
WORKDIR /app

COPY package.json bun.lock ./
RUN bun install --frozen-lockfile

COPY . .
RUN bun run build

FROM node:24-alpine
WORKDIR /app

COPY --from=build /app/build ./build

ENV NODE_ENV=production \
	HOST=0.0.0.0 \
	PORT=3000

USER node
EXPOSE 3000

CMD ["node", "build"]

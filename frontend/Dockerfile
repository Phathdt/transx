# React Router v8 SSR (Auth BFF). Build with pnpm; serve via @react-router/serve.
FROM node:24-alpine AS build

RUN corepack enable && corepack prepare pnpm@11.13.1 --activate

WORKDIR /app

COPY package.json pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile

COPY . .

# Browser-facing domain API base (Traefik on the host). Baked into the client bundle.
ARG VITE_API_BASE_URL=http://localhost:4000/api/v1
ENV VITE_API_BASE_URL=$VITE_API_BASE_URL

RUN pnpm build && pnpm prune --prod

FROM node:24-alpine AS runner

WORKDIR /app

ENV NODE_ENV=production \
    PORT=3000 \
    HOST=0.0.0.0

RUN addgroup -S nodejs && adduser -S rr -G nodejs

COPY --from=build --chown=rr:nodejs /app/package.json /app/pnpm-lock.yaml ./
COPY --from=build --chown=rr:nodejs /app/node_modules ./node_modules
COPY --from=build --chown=rr:nodejs /app/build ./build

USER rr

EXPOSE 3000

CMD ["./node_modules/.bin/react-router-serve", "./build/server/index.js"]

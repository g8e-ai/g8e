FROM node:22-alpine

LABEL description="Fleet Status Dashboard"

WORKDIR /app

COPY package.json ./
RUN npm install

COPY server.js ./
COPY public/ ./public/

EXPOSE 3000

CMD ["npx", "nodemon", "-w", "server.js", "-x", "node server.js"]

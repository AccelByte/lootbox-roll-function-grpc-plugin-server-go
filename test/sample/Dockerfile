FROM ngrok/ngrok as ngrok_image

FROM alpine:3.19
WORKDIR /workspace
RUN apk --no-cache add bash curl docker-cli-buildx git go jq make
COPY --from=ngrok_image /bin/ngrok /bin/ngrok
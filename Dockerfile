FROM alpine:3.19

# Install dependencies
RUN apk add --no-cache ca-certificates libc6-compat

# Create app directories
RUN mkdir -p /app/bin /app/plugins /app/config

# Copy binaries
COPY dist/bin/* /app/bin/
COPY dist/plugins/* /app/plugins/

# Copy documentation
COPY dist/README.md /app/
COPY dist/SHA256SUMS /app/

# Set environment variables
ENV PATH="/app/bin:${PATH}"
ENV FLOW_PLUGINS_DIR="/app/plugins"
ENV FLOW_CONFIG_DIR="/app/config"

# Set working directory
WORKDIR /app

# Set the entrypoint
ENTRYPOINT ["/app/bin/flow"]
CMD ["--help"] 
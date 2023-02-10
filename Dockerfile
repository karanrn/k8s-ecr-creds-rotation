ARG base_image
FROM ${base_image}

WORKDIR crons/k8s-ecr-creds-rotation
RUN ls

COPY ./ecr-creds-rotate /opt

ENTRYPOINT ["/opt/ecr-creds-rotate"]
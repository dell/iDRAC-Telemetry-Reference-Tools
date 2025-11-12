ARG USER_ID=1000
ARG GROUP_ID=1000
ARG USERNAME=telemetry
ARG GROUPNAME=telemetry
ARG CMD

FROM golang:1.24 as builder

ARG USER_ID
ARG GROUP_ID
ARG USERNAME
ARG GROUPNAME
ARG CMD

WORKDIR /app

RUN DEBIAN_FRONTEND=noninteractive apt-get update && DEBIAN_FRONTEND=noninteractive apt-get -y dist-upgrade

WORKDIR /build
# getent group ${GROUP_ID} - Get the name of the group. Ex: `somegroup:x:999:`. This makes sure the group exists
# echo groupdel by-id ${GROUP_ID} - This is just there to tell the user what we're doing
# groupdel $(getent group ${GROUP_ID} | cut -d: -f1) - Gets the group string as above (`somegroup:x:999:`)
# and then passes it into `cut -d: -f1` which will grab the `somegroup` part and pass that into groupdel
# ||: The || is the or operator and : is the null operator. This just has the effect of ensuring the line
# returns with "success" rather than failure in the event the group doesn't exist
RUN (getent group ${GROUP_ID}  && (echo groupdel by-id ${GROUP_ID}; groupdel $(getent group ${GROUP_ID} | cut -d: -f1))) ||:
RUN (getent group ${GROUPNAME} && (echo groupdel ${GROUPNAME}; groupdel ${GROUPNAME})) ||:
RUN (getent passwd ${USERNAME} && (echo userdel ${USERNAME}; userdel -f ${USERNAME})) ||:
RUN groupadd -g ${GROUP_ID} ${GROUPNAME}
RUN useradd -l -u ${USER_ID} -g ${GROUPNAME} ${USERNAME}

COPY internal /build/internal
COPY cmd/${CMD} /build/cmd/${CMD}
COPY go.* /build/

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o app ./cmd/${CMD}

FROM scratch

ARG CMD

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /etc/passwd /etc/group /etc/
COPY --from=builder /build/app /app

USER ${USERNAME}
ENTRYPOINT ["/app"]

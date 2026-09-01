FROM golang:1.22-bullseye

WORKDIR /app

# 1. Install Python 3, pip, and the venv module via Debian's package manager
RUN apt-get update && apt-get install -y python3 python3-pip python3-venv

# 2. Create the virtual environment and install the IMDb packages
RUN python3 -m venv venv
RUN ./venv/bin/pip install imdbinfo niquests

# 3. Copy your bot's code and build the executable
COPY . .
RUN go build -o filmigobot .

CMD ["./filmigobot"]

# Vina-GPU 2.1 docking engine.
# Build from source against Ubuntu 22.04/OpenCL so the resulting binary matches
# the worker runtime ABI used on the cluster runners and GPU nodes.

FROM ubuntu:22.04 AS builder

ARG DEBIAN_FRONTEND=noninteractive
ARG VINA_GPU_REPO_COMMIT=180272b8a5265d6ed9664178345933cebe2cd349
ARG VINA_GPU_METHOD=AutoDock-Vina-GPU-2.1
ARG VINA_GPU_BINARY=AutoDock-Vina-GPU-2-1
ARG BOOST_VERSION=1.84.0
ARG BOOST_VERSION_UNDERSCORE=1_84_0

RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential ca-certificates wget \
    ocl-icd-opencl-dev opencl-headers \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /tmp

RUN wget -q https://archives.boost.io/release/${BOOST_VERSION}/source/boost_${BOOST_VERSION_UNDERSCORE}.tar.gz \
    && tar -xzf boost_${BOOST_VERSION_UNDERSCORE}.tar.gz \
    && cd boost_${BOOST_VERSION_UNDERSCORE} \
    && ./bootstrap.sh --with-libraries=filesystem,program_options,system,thread \
    && ./b2 \
        --with-filesystem \
        --with-program_options \
        --with-system \
        --with-thread \
        variant=release \
        link=shared \
        threading=multi \
        stage

RUN wget -q -O vina-gpu.tar.gz https://github.com/DeltaGroupNJUPT/Vina-GPU-2.1/archive/${VINA_GPU_REPO_COMMIT}.tar.gz \
    && tar -xzf vina-gpu.tar.gz \
    && mv Vina-GPU-2.1-${VINA_GPU_REPO_COMMIT} /src-vina-gpu

WORKDIR /src-vina-gpu/${VINA_GPU_METHOD}

RUN g++ -std=c++17 -O3 -DNDEBUG \
    -DOPENCL_3_0 -DNVIDIA_PLATFORM -DLARGE_BOX -DBOOST_TIMER_ENABLE_DEPRECATED -DBUILD_KERNEL_FROM_SOURCE \
    -I/tmp/boost_${BOOST_VERSION_UNDERSCORE} \
    -I/tmp/boost_${BOOST_VERSION_UNDERSCORE}/boost \
    -I./lib -I./OpenCL/inc -I/usr/include \
    ./main/main.cpp ./lib/*.cpp ./OpenCL/src/wrapcl.cpp \
    /tmp/boost_${BOOST_VERSION_UNDERSCORE}/libs/thread/src/pthread/thread.cpp \
    /tmp/boost_${BOOST_VERSION_UNDERSCORE}/libs/thread/src/pthread/once.cpp \
    -L/tmp/boost_${BOOST_VERSION_UNDERSCORE}/stage/lib -L/usr/lib/x86_64-linux-gnu \
    -lboost_program_options -lboost_system -lboost_filesystem -lOpenCL -lm -lpthread -lstdc++fs \
    -o /opt/AutoDock-Vina-GPU-2-1

FROM ubuntu:22.04

ARG DEBIAN_FRONTEND=noninteractive
ARG BOOST_VERSION_UNDERSCORE=1_84_0
ARG VINA_GPU_METHOD=AutoDock-Vina-GPU-2.1

ENV PYTHONUNBUFFERED=1 \
    VINA_GPU_BIN=/opt/vina-gpu/bin/AutoDock-Vina-GPU-2-1 \
    VINA_GPU_OPENCL_BINARY_PATH=/opt/vina-gpu/bin \
    LD_LIBRARY_PATH=/run/opengl-driver/lib:/opt/boost/lib:/usr/lib/x86_64-linux-gnu

WORKDIR /app

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates python3 python3-pip libgomp1 ocl-icd-libopencl1 \
    && rm -rf /var/lib/apt/lists/*

RUN mkdir -p /opt/vina-gpu/bin /opt/boost
COPY --from=builder /opt/AutoDock-Vina-GPU-2-1 /opt/vina-gpu/bin/AutoDock-Vina-GPU-2-1
COPY --from=builder /tmp/boost_${BOOST_VERSION_UNDERSCORE}/stage/lib /opt/boost/lib
RUN ln -sf /opt/vina-gpu/bin/AutoDock-Vina-GPU-2-1 /usr/local/bin/vina-gpu

COPY requirements.txt ./
RUN pip3 install --no-cache-dir -r requirements.txt

COPY scripts/ /scripts/
COPY app.py ./

CMD ["python3", "/scripts/dock_batch.py"]

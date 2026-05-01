# Vina-GPU 2.1 docking engine.
# Build from source against Ubuntu 22.04/OpenCL so the resulting binary matches
# the worker runtime ABI used on the cluster runners and GPU nodes.
#
# Two binaries are produced:
#   warmup  (-DBUILD_KERNEL_FROM_SOURCE): compiles OpenCL kernels and saves
#            Kernel1_Opt.bin / Kernel2_Opt.bin on first run.
#   runtime (no flag): loads pre-compiled .bin files directly.

FROM ubuntu:22.04 AS builder

ARG DEBIAN_FRONTEND=noninteractive
ARG VINA_GPU_REPO_COMMIT=180272b8a5265d6ed9664178345933cebe2cd349
ARG VINA_GPU_METHOD=AutoDock-Vina-GPU-2.1
ARG BOOST_VERSION=1.84.0
ARG BOOST_VERSION_UNDERSCORE=1_84_0
ARG VINA_GPU_RUNTIME_ARTIFACT=/tmp/khemeia-vina-gpu-runtime.bin
ARG VINA_GPU_WARMUP_ARTIFACT=/tmp/khemeia-vina-gpu-warmup.bin

RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential ca-certificates wget python3 \
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

# Lower the minimum thread count (upstream default 1000 is too high for single-GPU).
RUN sed -i 's/thread < 1000/thread < 1/' ./main/main.cpp \
    && sed -i 's/thread must be at least 1000/thread must be at least 1/' ./main/main.cpp

# Patch main_procedure_cl.cpp:
#   - Use opencl_binary_path for kernel source/binary paths instead of ".".
#   - Clear the source concatenation buffer between Kernel1 and Kernel2 to
#     prevent stale data leaking across compilations.
#   - Add binary-load path in the runtime (#else) branch.
COPY build/patch_main_procedure.py /tmp/patch_main_procedure.py
RUN python3 /tmp/patch_main_procedure.py

# Warmup binary: compiles OpenCL kernels from source and saves .bin files.
RUN g++ -std=c++17 -O3 -DNDEBUG \
    -DOPENCL_3_0 -DNVIDIA_PLATFORM -DSMALL_BOX -DBOOST_TIMER_ENABLE_DEPRECATED \
    -DBUILD_KERNEL_FROM_SOURCE \
    -I/tmp/boost_${BOOST_VERSION_UNDERSCORE} \
    -I/tmp/boost_${BOOST_VERSION_UNDERSCORE}/boost \
    -I./lib -I./OpenCL/inc -I/usr/include \
    ./main/main.cpp ./lib/*.cpp ./OpenCL/src/wrapcl.cpp \
    /tmp/boost_${BOOST_VERSION_UNDERSCORE}/libs/thread/src/pthread/thread.cpp \
    /tmp/boost_${BOOST_VERSION_UNDERSCORE}/libs/thread/src/pthread/once.cpp \
    -L/tmp/boost_${BOOST_VERSION_UNDERSCORE}/stage/lib -L/usr/lib/x86_64-linux-gnu \
    -lboost_program_options -lboost_system -lboost_filesystem -lOpenCL -lm -lpthread -lstdc++fs \
    -o ${VINA_GPU_WARMUP_ARTIFACT}

# Runtime binary: loads pre-compiled .bin files (no JIT kernel compilation).
RUN g++ -std=c++17 -O3 -DNDEBUG \
    -DOPENCL_3_0 -DNVIDIA_PLATFORM -DSMALL_BOX -DBOOST_TIMER_ENABLE_DEPRECATED \
    -I/tmp/boost_${BOOST_VERSION_UNDERSCORE} \
    -I/tmp/boost_${BOOST_VERSION_UNDERSCORE}/boost \
    -I./lib -I./OpenCL/inc -I/usr/include \
    ./main/main.cpp ./lib/*.cpp ./OpenCL/src/wrapcl.cpp \
    /tmp/boost_${BOOST_VERSION_UNDERSCORE}/libs/thread/src/pthread/thread.cpp \
    /tmp/boost_${BOOST_VERSION_UNDERSCORE}/libs/thread/src/pthread/once.cpp \
    -L/tmp/boost_${BOOST_VERSION_UNDERSCORE}/stage/lib -L/usr/lib/x86_64-linux-gnu \
    -lboost_program_options -lboost_system -lboost_filesystem -lOpenCL -lm -lpthread -lstdc++fs \
    -o ${VINA_GPU_RUNTIME_ARTIFACT}

# ---- Runtime image ----
FROM ubuntu:22.04

ARG DEBIAN_FRONTEND=noninteractive
ARG BOOST_VERSION_UNDERSCORE=1_84_0
ARG VINA_GPU_METHOD=AutoDock-Vina-GPU-2.1
ARG VINA_GPU_RUNTIME_ARTIFACT=/tmp/khemeia-vina-gpu-runtime.bin
ARG VINA_GPU_WARMUP_ARTIFACT=/tmp/khemeia-vina-gpu-warmup.bin

ENV PYTHONUNBUFFERED=1 \
    VINA_GPU_BIN=/opt/vina-gpu/bin/AutoDock-Vina-GPU-2-1 \
    VINA_GPU_WARMUP_BIN=/opt/vina-gpu/bin/AutoDock-Vina-GPU-2-1-source \
    VINA_GPU_OPENCL_BINARY_PATH=/opt/vina-gpu/bin \
    LD_LIBRARY_PATH=/run/opengl-driver/lib:/opt/boost/lib:/usr/lib/x86_64-linux-gnu

WORKDIR /app

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates python3 python3-pip libgomp1 ocl-icd-libopencl1 \
    openbabel \
    && rm -rf /var/lib/apt/lists/*

RUN mkdir -p /opt/vina-gpu/bin /opt/boost

COPY --from=builder ${VINA_GPU_RUNTIME_ARTIFACT} /opt/vina-gpu/bin/AutoDock-Vina-GPU-2-1
COPY --from=builder ${VINA_GPU_WARMUP_ARTIFACT}  /opt/vina-gpu/bin/AutoDock-Vina-GPU-2-1-source
COPY --from=builder /src-vina-gpu/${VINA_GPU_METHOD}/OpenCL /opt/vina-gpu/bin/OpenCL
COPY --from=builder /tmp/boost_${BOOST_VERSION_UNDERSCORE}/stage/lib /opt/boost/lib

RUN chmod 0755 /opt/vina-gpu/bin/AutoDock-Vina-GPU-2-1 \
               /opt/vina-gpu/bin/AutoDock-Vina-GPU-2-1-source \
    && ln -sf /opt/vina-gpu/bin/AutoDock-Vina-GPU-2-1 /usr/local/bin/vina-gpu

COPY requirements.txt ./
RUN pip3 install --no-cache-dir -r requirements.txt

COPY scripts/ /scripts/
COPY app.py ./

CMD ["python3", "/scripts/dock_batch.py"]

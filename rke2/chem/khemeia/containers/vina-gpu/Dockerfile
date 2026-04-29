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

RUN sed -i 's/thread < 1000/thread < 1/' ./main/main.cpp \
    && sed -i 's/thread must be at least 1000/thread must be at least 1/' ./main/main.cpp

RUN python3 - <<'PY'
from pathlib import Path
path = Path("/src-vina-gpu/AutoDock-Vina-GPU-2.1/lib/main_procedure_cl.cpp")
text = path.read_text()
old = """#ifdef BUILD_KERNEL_FROM_SOURCE
\tconst std::string default_work_path = \".\";
\tconst std::string include_path = default_work_path + \"/OpenCL/inc\"; //FIX it
\tconst std::string addtion = \"\";

\tprintf(\"\\n\\nBuild kernel 1 from source\"); fflush(stdout);
\tchar* program1_file_n[NUM_OF_FILES_KERNEL_1];
\tsize_t program1_size_n[NUM_OF_FILES_KERNEL_1];
\tstd::string file1_paths[NUM_OF_FILES_KERNEL_1] = {\tdefault_work_path + \"/OpenCL/src/kernels/code_head.cl\",
\t\t\t\t\t\t\t\t\t\t\t\t//default_work_path + \"/OpenCL/src/kernels/mutate_conf.cl\",
\t\t\t\t\t\t\t\t\t\t\t\t//default_work_path + \"/OpenCL/src/kernels/matrix.cl\",
\t\t\t\t\t\t\t\t\t\t\t\t//default_work_path + \"/OpenCL/src/kernels/quasi_newton.cl\",
\t\t\t\t\t\t\t\t\t\t\t\tdefault_work_path + \"/OpenCL/src/kernels/kernel1.cl\" }; // The order of files is important!

\tread_n_file(program1_file_n, program1_size_n, file1_paths, NUM_OF_FILES_KERNEL_1);
\tstd::string final_file;
\tsize_t final_size = NUM_OF_FILES_KERNEL_1 - 1; // count '\\n'
\tfor (int i = 0; i < NUM_OF_FILES_KERNEL_1; i++) {
\t\tif (i == 0) final_file = program1_file_n[0];
\t\telse final_file = final_file + '\\n' + (std::string)program1_file_n[i];
\t\tfinal_size += program1_size_n[i];
\t}
\tconst char* final_files1_char = final_file.data();

\tprograms[0] = clCreateProgramWithSource(context, 1, (const char**)&final_files1_char, &final_size, &err); checkErr(err);
\tSetupBuildProgramWithSource(programs[0], NULL, devices, include_path, addtion);
\tSaveProgramToBinary(programs[0], \"Kernel1_Opt.bin\");


\tprintf(\"\\nBuild kernel 2 from source\"); fflush(stdout);
\tchar* program2_file_n[NUM_OF_FILES_KERNEL_2];
\tsize_t program2_size_n[NUM_OF_FILES_KERNEL_2];
\tstd::string file2_paths[NUM_OF_FILES_KERNEL_2] = { default_work_path + \"/OpenCL/src/kernels/code_head.cl\",
\t\t\t\t\t\t\t\t\t\t\t   default_work_path + \"/OpenCL/src/kernels/mutate_conf.cl\",
\t\t\t\t\t\t\t\t\t\t\t   default_work_path + \"/OpenCL/src/kernels/matrix.cl\",
\t\t\t\t\t\t\t\t\t\t\t   default_work_path + \"/OpenCL/src/kernels/quasi_newton.cl\",
\t\t\t\t\t\t\t\t\t\t\t   default_work_path + \"/OpenCL/src/kernels/kernel2.cl\" }; // The order of files is important!

\tread_n_file(program2_file_n, program2_size_n, file2_paths, NUM_OF_FILES_KERNEL_2);
\tfinal_size = NUM_OF_FILES_KERNEL_2 - 1; // count '\\n'
\tfor (int i = 0; i < NUM_OF_FILES_KERNEL_2; i++) {
\t\tif (i == 0) final_file = program2_file_n[0];
\t\telse final_file = final_file + '\\n' + (std::string)program2_file_n[i];
\t\tfinal_size += program2_size_n[i];
\t}
\tconst char* final_files2_char = final_file.data();

\tprograms[1] = clCreateProgramWithSource(context, 1, (const char**)&final_files2_char, &final_size, &err); checkErr(err);
\tSetupBuildProgramWithSource(programs[1], NULL, devices, include_path, addtion);
\tSaveProgramToBinary(programs[1], \"Kernel2_Opt.bin\");
#endif
\tprograms[0] = SetupBuildProgramWithBinary(context, devices, (opencl_binary_path + std::string(\"/Kernel1_Opt.bin\")).c_str());

\tprograms[1] = SetupBuildProgramWithBinary(context, devices, (opencl_binary_path + std::string(\"/Kernel2_Opt.bin\")).c_str());
"""
new = """#ifdef BUILD_KERNEL_FROM_SOURCE
\tconst std::string default_work_path = opencl_binary_path;
\tconst std::string include_path = default_work_path + \"/OpenCL/inc\";
\tconst std::string addtion = \"\";

\tprintf(\"\\n\\nBuild kernel 1 from source\"); fflush(stdout);
\tchar* program1_file_n[NUM_OF_FILES_KERNEL_1];
\tsize_t program1_size_n[NUM_OF_FILES_KERNEL_1];
\tstd::string file1_paths[NUM_OF_FILES_KERNEL_1] = {\tdefault_work_path + \"/OpenCL/src/kernels/code_head.cl\",
\t\t\t\t\t\t\t\t\t\t\t\tdefault_work_path + \"/OpenCL/src/kernels/kernel1.cl\" };

\tread_n_file(program1_file_n, program1_size_n, file1_paths, NUM_OF_FILES_KERNEL_1);
\tstd::string final_file;
\tsize_t final_size = NUM_OF_FILES_KERNEL_1 - 1;
\tfor (int i = 0; i < NUM_OF_FILES_KERNEL_1; i++) {
\t\tif (i == 0) final_file = program1_file_n[0];
\t\telse final_file = final_file + '\\n' + (std::string)program1_file_n[i];
\t\tfinal_size += program1_size_n[i];
\t}
\tconst char* final_files1_char = final_file.data();

\tprograms[0] = clCreateProgramWithSource(context, 1, (const char**)&final_files1_char, &final_size, &err); checkErr(err);
\tSetupBuildProgramWithSource(programs[0], NULL, devices, include_path, addtion);
\tSaveProgramToBinary(programs[0], (opencl_binary_path + std::string(\"/Kernel1_Opt.bin\")).c_str());

\tprintf(\"\\nBuild kernel 2 from source\"); fflush(stdout);
\tchar* program2_file_n[NUM_OF_FILES_KERNEL_2];
\tsize_t program2_size_n[NUM_OF_FILES_KERNEL_2];
\tstd::string file2_paths[NUM_OF_FILES_KERNEL_2] = { default_work_path + \"/OpenCL/src/kernels/code_head.cl\",
\t\t\t\t\t\t\t\t\t\t\t   default_work_path + \"/OpenCL/src/kernels/mutate_conf.cl\",
\t\t\t\t\t\t\t\t\t\t\t   default_work_path + \"/OpenCL/src/kernels/matrix.cl\",
\t\t\t\t\t\t\t\t\t\t\t   default_work_path + \"/OpenCL/src/kernels/quasi_newton.cl\",
\t\t\t\t\t\t\t\t\t\t\t   default_work_path + \"/OpenCL/src/kernels/kernel2.cl\" };

\tread_n_file(program2_file_n, program2_size_n, file2_paths, NUM_OF_FILES_KERNEL_2);
\tfinal_file.clear();
\tfinal_size = NUM_OF_FILES_KERNEL_2 - 1;
\tfor (int i = 0; i < NUM_OF_FILES_KERNEL_2; i++) {
\t\tif (i == 0) final_file = program2_file_n[0];
\t\telse final_file = final_file + '\\n' + (std::string)program2_file_n[i];
\t\tfinal_size += program2_size_n[i];
\t}
\tconst char* final_files2_char = final_file.data();

\tprograms[1] = clCreateProgramWithSource(context, 1, (const char**)&final_files2_char, &final_size, &err); checkErr(err);
\tSetupBuildProgramWithSource(programs[1], NULL, devices, include_path, addtion);
\tSaveProgramToBinary(programs[1], (opencl_binary_path + std::string(\"/Kernel2_Opt.bin\")).c_str());
#else
\tprograms[0] = SetupBuildProgramWithBinary(context, devices, (opencl_binary_path + std::string(\"/Kernel1_Opt.bin\")).c_str());
\tprograms[1] = SetupBuildProgramWithBinary(context, devices, (opencl_binary_path + std::string(\"/Kernel2_Opt.bin\")).c_str());
#endif
"""
if old not in text:
    raise SystemExit("expected upstream block not found")
path.write_text(text.replace(old, new))
PY

RUN g++ -std=c++17 -O3 -DNDEBUG \
    -DOPENCL_3_0 -DNVIDIA_PLATFORM -DSMALL_BOX -DBOOST_TIMER_ENABLE_DEPRECATED -DBUILD_KERNEL_FROM_SOURCE \
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
COPY --from=builder /src-vina-gpu/${VINA_GPU_METHOD}/OpenCL /opt/vina-gpu/bin/OpenCL
COPY --from=builder /tmp/boost_${BOOST_VERSION_UNDERSCORE}/stage/lib /opt/boost/lib
RUN ln -sf /opt/vina-gpu/bin/AutoDock-Vina-GPU-2-1 /usr/local/bin/vina-gpu

COPY requirements.txt ./
RUN pip3 install --no-cache-dir -r requirements.txt

COPY scripts/ /scripts/
COPY app.py ./

CMD ["python3", "/scripts/dock_batch.py"]

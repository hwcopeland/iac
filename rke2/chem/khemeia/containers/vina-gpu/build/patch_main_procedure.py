#!/usr/bin/env python3
"""Patch Vina-GPU 2.1 main_procedure_cl.cpp to fix OpenCL kernel path handling.

Changes applied:
- Replace hardcoded "." path with opencl_binary_path variable.
- Clear final_file between Kernel1 and Kernel2 compilation to avoid stale
  string data accumulating in the concatenation buffer.
- Add fallback binary-load path in the #else branch (runtime binary).
"""

from pathlib import Path

SRC = Path("/src-vina-gpu/AutoDock-Vina-GPU-2.1/lib/main_procedure_cl.cpp")

MARKER_START = "#ifdef BUILD_KERNEL_FROM_SOURCE"
MARKER_END = "\terr = clUnloadPlatformCompiler(platforms[gpu_platform_id]); checkErr(err);"

REPLACEMENT = r"""#ifdef BUILD_KERNEL_FROM_SOURCE
	const std::string default_work_path = opencl_binary_path;
	const std::string include_path = default_work_path + "/OpenCL/inc";
	const std::string addtion = "";

	printf("\n\nBuild kernel 1 from source"); fflush(stdout);
	char* program1_file_n[NUM_OF_FILES_KERNEL_1];
	size_t program1_size_n[NUM_OF_FILES_KERNEL_1];
	std::string file1_paths[NUM_OF_FILES_KERNEL_1] = {	default_work_path + "/OpenCL/src/kernels/code_head.cl",
												default_work_path + "/OpenCL/src/kernels/kernel1.cl" };

	read_n_file(program1_file_n, program1_size_n, file1_paths, NUM_OF_FILES_KERNEL_1);
	std::string final_file;
	size_t final_size = NUM_OF_FILES_KERNEL_1 - 1;
	for (int i = 0; i < NUM_OF_FILES_KERNEL_1; i++) {
		if (i == 0) final_file = program1_file_n[0];
		else final_file = final_file + '\n' + (std::string)program1_file_n[i];
		final_size += program1_size_n[i];
	}
	const char* final_files1_char = final_file.data();

	programs[0] = clCreateProgramWithSource(context, 1, (const char**)&final_files1_char, &final_size, &err); checkErr(err);
	SetupBuildProgramWithSource(programs[0], NULL, devices, include_path, addtion);
	SaveProgramToBinary(programs[0], (opencl_binary_path + std::string("/Kernel1_Opt.bin")).c_str());

	printf("\nBuild kernel 2 from source"); fflush(stdout);
	char* program2_file_n[NUM_OF_FILES_KERNEL_2];
	size_t program2_size_n[NUM_OF_FILES_KERNEL_2];
	std::string file2_paths[NUM_OF_FILES_KERNEL_2] = { default_work_path + "/OpenCL/src/kernels/code_head.cl",
												   default_work_path + "/OpenCL/src/kernels/mutate_conf.cl",
												   default_work_path + "/OpenCL/src/kernels/matrix.cl",
												   default_work_path + "/OpenCL/src/kernels/quasi_newton.cl",
												   default_work_path + "/OpenCL/src/kernels/kernel2.cl" };

	read_n_file(program2_file_n, program2_size_n, file2_paths, NUM_OF_FILES_KERNEL_2);
	final_file.clear();
	final_size = NUM_OF_FILES_KERNEL_2 - 1;
	for (int i = 0; i < NUM_OF_FILES_KERNEL_2; i++) {
		if (i == 0) final_file = program2_file_n[0];
		else final_file = final_file + '\n' + (std::string)program2_file_n[i];
		final_size += program2_size_n[i];
	}
	const char* final_files2_char = final_file.data();

	programs[1] = clCreateProgramWithSource(context, 1, (const char**)&final_files2_char, &final_size, &err); checkErr(err);
	SetupBuildProgramWithSource(programs[1], NULL, devices, include_path, addtion);
	SaveProgramToBinary(programs[1], (opencl_binary_path + std::string("/Kernel2_Opt.bin")).c_str());
#else
	programs[0] = SetupBuildProgramWithBinary(context, devices, (opencl_binary_path + std::string("/Kernel1_Opt.bin")).c_str());
	programs[1] = SetupBuildProgramWithBinary(context, devices, (opencl_binary_path + std::string("/Kernel2_Opt.bin")).c_str());
#endif
"""

text = SRC.read_text()

start = text.index(MARKER_START)
end = text.index(MARKER_END)

SRC.write_text(text[:start] + REPLACEMENT + text[end:])
print(f"Patched {SRC}")

#
# Makefile for program DFRATOM
# Written by Anatoliy Volkov (c) MTSU, 2025
#

FC = gfortran-14
#FC = gfortran-9
#FC = gfortran-7
#FC = lfc
#FC = flang
#FC = g95

### Numerical precision of the code in bits; default is 64 bits
#PRECISION = 
#PRECISION = -DUSE_BINARY128

# gfortran
FOPT = -fallow-argument-mismatch -mcmodel=large -O2
#FOPT = -fallow-argument-mismatch -mcmodel=large -g -fbounds-check -frange-check -fbacktrace
#FOPT = -fallow-argument-mismatch -mcmodel=large -O0 -g -fbounds-check -frange-check -fbacktrace -Wall  

### LAPACK libraries
#LAPACK =
LAPACK =  -llapack -lblas
#LIBDIR = /usr/local/ATLAS/lib
#LAPACK = $(LIBDIR)/liblapack.a  $(LIBDIR)/libcblas.a $(LIBDIR)/libf77blas.a $(LIBDIR)/libatlas.a

#LDFLAGS = $(FOPT)

# FMSAVE.o FMZM90.o FM.o
# HYPER1F1.o set_rk.o Faddeyeva_v3_mod_rk.o chgm.o 


DFRATOM:  DFRATOM.o 
	$(FC) DFRATOM.f -o DFRATOM.exe $(FOPT) $(LAPACK)

clean:
	rm -rf DFRATOM.exe $(OBJS) *.mod

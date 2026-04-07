cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc
c
c GNU Fortran:
c gfortran-14 DFRATOM.f -o DFRATOM.exe -fallow-argument-mismatch -mcmodel=large -O2 -llapack
c gfortran-14 DFRATOM.f -o DFRATOM.exe -fallow-argument-mismatch -mcmodel=large -g -fbounds-check -frange-check -fbacktrace -llapack
c gfortran-14 DFRATOM.f -o DFRATOM.exe -fallow-argument-mismatch -mcmodel=large -O0 -g -fbounds-check -frange-check -fbacktrace -Wall -llapack   
c gfortran-7  DFRATOM.f -o DFRATOM.exe -mcmodel=large -g -fbounds-check -frange-check -fbacktrace -Wline-truncation -llapack
c
c Oracle studio
c sunf90 DFRATOM.f -o DFRATOM -C -e -r8const -xcheck=%all
c
c AOCC/FLANG:
c flang DFRATOM.f -o DFRATOM -fdefault-real-8
c
c Intel:
c ifx DFRATOM.f -o DFRATOM -O0 -g -check all -real-size 64 -warn all
c
      !***********************************************************************

      module Factorials

      integer*4 :: nfact
      Real*8, allocatable :: fact(:)

      contains
            
      subroutine AllocateFactorials(nfact)
      integer error
      if( allocated(fact) ) call DeallocateFactorials
      allocate( fact(0:nfact), stat = error )
      if( error > 0 ) stop 'AllocateFactorials :: Mem. alloc. error'
      fact(0) = 1.00D00
      do i = 1, nfact
        fact(i) = fact(i-1)*dble(i)
c        write(*,'(i4,1p,e20.10)') i, fact(i)
      enddo
      end subroutine AllocateFactorials
      
      subroutine DeallocateFactorials
      integer :: error
      deallocate( fact, stat = error)
      if( error > 0 ) stop 'DeallocateFactorials :: Mem. dealloc. error'
      end subroutine DeallocateFactorials

      end module Factorials

cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc

      module DoubleFactorials

      integer*4 :: ndfac
      Real*8, allocatable :: dfac(:)

      contains
            
      subroutine AllocateDoubleFactorials(ndfac)
      integer error
      if( allocated(dfac) ) call DeallocateDoubleFactorials
      allocate( dfac(-1:ndfac), stat = error )
      if( error > 0) 
     & stop 'AllocateDoubleFactorials :: Mem. alloc. error'

      dfac = 1.00D00

      do i=1,ndfac
        do j=i,1,-2
          dfac(i) = dfac(i)*dble(j)
        enddo  
      enddo 

      end subroutine AllocateDoubleFactorials
      
      subroutine DeallocateDoubleFactorials
      integer :: error
      deallocate( dfac, stat = error)
      if( error > 0 ) 
     & stop 'DeallocateDoubleFactorials :: Mem. dealloc. error' 
      end subroutine DeallocateDoubleFactorials

      end module DoubleFactorials

      !***********************************************************************
c
c ADOK 
c DFRATOM.  AN ATOMIC DIRAC-FOCK-ROOTHAAN PROGRAM.  O. MATSUOKA, Y. WATANABE.                                                   
c REF. IN COMP. PHYS. COMMUN. 139 (2001) 218                              
C-----------------------------------------------------------------------
      PROGRAM DFRATM
C     PROPHET-DFR-ATOM (CGTF)
C
C     ATOMIC DIRAC-FOCK-ROOTHAAN SCF PROGRAM
C     FOR GENERALIZED AVERAGE ENERGY OF CONFIGURATIONS
C     USING CONTRACTED GAUSSIAN-TYPE BASIS FUNCTIONS
C
C     VERSION 1.0 FOR CPC LIBRARY (MARCH 3, 2000)
C
C     OSAMU MATSUOKA AND YOSHIHIRO WATANABE
C     DEPARTMENT OF CHEMISTRY
C     KYUSHU UNIVERSITY
C     ROPPONMATSU, FUKUOKA 810-8560, JAPAN
C-----------------------------------------------------------------------
C
cAV
      use Factorials
      use DoubleFactorials
cendAV
      IMPLICIT REAL*8 (A-H,O-Z)
C
C**********************************************************************
      PARAMETER (N1=90,N5=18,N6=8,N7=60 000 000,N8=20,
     1  NSM=7,N2=N1*(N1*2+1)*NSM,N4=(N1*2)**2*NSM)
      PARAMETER (NXBUFF=0500,NFTXP=20)
C
C        N1 .GE. MAX(NBS(L),L=1,NSYM)
C        N2 .GE. SUM(L=1,NSYM) NBS(L)*(NBS(L)*2+1)
C                = NUMBER OF ELEMENTS OF S-, H-, OR F-MATRIX
C        N4 .GE. SUM(L=1,NSYM) (NBS(L)*2)**2
C                = NUMBER OF COEFFICIENTS OF ATOMIC SPINORS
C        N5 .GE. MAX((NPBS(P,L),P=1,NBS(L)),L=1,NSYM))
C        N6 .GE. MAX(NOS(L),L=1,NSYM)
C        N7 .GE. N*(N+1)/2;  N=SUM(L=1,NSYM) NBS(L)*(NBS(L)+1)/2
C        N8 .GE. NCONF
C
C     WHERE NSYM   = NUMBER OF SYMMETRY SPECIES
C           NBS(L) = NUMBER OF BASIS SPINORS OF THE L-TH SYMM SPECIES
C           NPBS(P,L) = NUMBER OF PRIMITIVE BASIS SPINORS OF THE P-TH
C                        BASIS SPINOR OF THE L-TH SYMM SPECIES
C           NOS(L) = NUMBER OF OCCUPIED SPINORS OF THE L-TH SYMM SPECIES
C           NCONF = NO OF CONFIGURATIONS FOR AVERAGE ENERGY
C
C           VIOLATION OF THESE PRARAMETER VALUES IS CHECKED BY
C           SUBROUTINE "SPACE" AND WILL BE FOUND IN THE PRINT
C           "STORAGE SPACES REQUIRED AND GIVEN" AFTER THE PLAYBACK
C           OF INPUT DATA.
C
C     WHEN P-SUPERMATRIX IS TO BE STORED IN FAST MEMORIES,
C     REQUIRED STORAGE SPACES ARE ALMOST DETERMINED BY
C     THE NUMBER OF ITS ELEMENTS:
C             N7*8 DOUBLE WORDS = N7*64 BYTES
C     WHILE, WHEN IT IS TO BE STORED IN DISK FILE, ONE MAY SET N7=0,
C     (OR ANY SMALL NUMBER) AND ASSIGN APPROPRIATE VALUES FOR NFTXP
C     (DISK FILE #) AND NXBUFF (NUMBER OF ELEMENTS OF P-SUPERMATRIX
C     TO BE BROUGHT INTO FAST MEMORIES BY ONE TRANSFER).
C***********************************************************************
C
      CHARACTER*8 TITLE(20)
      DIMENSION NBS(7),NOS(3,7),OCUP(2,7),VCC(7,7)
      DIMENSION WAV(N8),OCUPAV(7,N8)
      DIMENSION NPBS(N1,7),ZETA(N5,N1,7),CBS(N5,N1,2,7)
      DIMENSION EV(N1*2,7),VC(N4),REXP(-2:4,N6,7)
      DIMENSION NTERM(2,7),NQNTM(2,2,7)
      DIMENSION COEFB(0:6,7,7),NUMIN(7,7),NUMAX(7,7)
      DIMENSION CNORM(2,N5,N1,2,7)
      DIMENSION SMX(N2),HMX(N2),XP(8,N7)
      DIMENSION XPBUFF(NXBUFF*8)
      DIMENSION LOCMX(7),LOCTR(2*N1),LOCXP(7),LOCVC(7)
      DIMENSION DTMX(N2),DOMX(N2),FCMX(N2),FOMX(N2)
      DIMENSION VCM1(N4),DTMXM1(N2),FCMXM1(N2),FOMXM1(N2)
      DIMENSION WVEC(N1*2),WMX1(N1*2,N1*2),WMX2(N1*2,N1*2),
     1  WMX3(N1*2,N1*2),WMX4(N1*2,6),IW(N1*2)
c**    EQUIVALENCE (PMX,FCMX),(QMX,FOMX)
C
      COMMON /FNUT/F(0:8),TABF(0:14,0:120)

cAV
      nfact = 170;  call AllocateFactorials( nfact )
      ndfac =  20;  call AllocateDoubleFactorials( ndfac )
cendAV

C.... PRINT HEADING TITLE
      WRITE(6,10)
   10 FORMAT(79('-')/'PROPHET-DFR-ATOM (CGTF)'//
     1  'ATOMIC DIRAC-FOCK-ROOTHAAN SCF PROGRAM'/
     2  'FOR GENERALIZED AVERAGE ENERGY OF CONFIGURATIONS')
C
C.... PREPARE FOR COMPUTATION
      CALL INTIN(TITLE,  ZNUC,RNUC,ALPHA,NUCMDL,  C,
     1  NSYM,NBS,NPBS,ZETA,CBS,N1,N5,  NTERM,NQNTM,  MTDPMX,
     2  NMX,  LOCMX,LOCTR,LOCXP,  COEFB,NUMIN,NUMAX)
      CALL SCFIN(NSYM,NOS,  NCONF,WAV,OCUPAV,
     1  MXITR,THCVL,THCVS,THCVEN,THDLL,THDSL,THDSS,
     2  IXTRP,DFCTR,  IPRVC,IPRMX,INTLVC,  VC,  NFTSCF,
     3  OCUP,VCC,  NBS,  NVC,LOCVC)
C
      WRITE(6,12)
   12 FORMAT(/79('-'))
C
      CALL SPACE(N1,N2,N4,N5,N6,N7,N8,
     1  NSYM,NBS,NPBS,NOS,NCONF,  MTDPMX,  *400)
C
      IF(NUCMDL.EQ.2) CALL FTABLE
C
      CALL BSNORM(CNORM,  NSYM,NBS,NPBS,ZETA,N1,N5)
      CALL NORMBS(CBS,N1,N5,  NSYM,NBS,NPBS,ZETA,  NTERM,NQNTM,CNORM)
C
      IERR=0
C
C.... COMPUTE ENERGY INTEGRALS
      CALL EINT1(SMX,HMX,
     1  NSYM,NBS,NPBS,ZETA,CBS,N1,N5,
     2  NTERM,NQNTM,CNORM,  ZNUC,RNUC,ALPHA,NUCMDL,  C,  LOCMX,LOCTR)
C
      CALL EINT2(XP,  XPBUFF,NXBUFF,
     1  NFTXP,  MTDPMX,  NSYM,NBS,NPBS,ZETA,CBS,N1,N5,
     2  NTERM,NQNTM,CNORM,  COEFB,NUMIN,NUMAX,  LOCXP,LOCTR)
C
C.... GUESS INITIAL C-VECTORS AND IMPLEMENT SCF ITERATIONS
      IF(INTLVC.EQ.1) CALL GUESS(VC,  NSYM,NBS,NOS,
     1  LOCVC,LOCMX,  SMX,HMX,
     2  WVEC,WMX1,WMX2,WMX3,WMX4,IW,  *400)
C
      IERR=0
      CALL ITERAT(EV,VC,N1*2,  EMASS,EKIN,EPOT,ETOT,
     1  DIFVCL,DIFVCS,DIFFEN,DIFDLL,DIFDSL,DIFDSS,
     2  NSYM,NBS,NOS,OCUP,  VCC,  C,
     3  MXITR,THCVL,THCVS,THCVEN,THDLL,THDSL,THDSS,ICONV,
     4  IXTRP,DFCTR,  LOCVC,LOCMX,LOCTR,
c**   5  NMX,  SMX,HMX,  DTMX,DOMX,  PMX,QMX,  FCMX,FOMX,
     5  NMX,  SMX,HMX,  DTMX,DOMX,  FCMX,FOMX,  FCMX,FOMX,
     6  NVC,VCM1,  DTMXM1,FCMXM1,FOMXM1,  MTDPMX,
     7  XP,  XPBUFF,NXBUFF,NFTXP,
     8  WMX1,WVEC,WMX2,WMX3,WMX4,IW,  ITER,  IERR,  *400)
C
C.... PRINT RESULTS
      CALL SCFOUT(EV,VC,N1*2,  ETOT,EMASS,EKIN,EPOT,  C,
     1  TITLE,  ZNUC,RNUC,ALPHA,NUCMDL,  NSYM,NBS,NOS, NCONF,WAV,
     2  OCUPAV,DIFVCL,DIFVCS,DIFFEN,DIFDLL,DIFDSL,DIFDSS,  ICONV,ITER,
     3  LOCVC,  IPRVC,IPRMX,SMX,HMX,DTMX,DOMX,FCMX,FOMX,  LOCMX,LOCTR,
     4  REXP,N6,  NPBS,ZETA,CBS,N1,N5,  NTERM,NQNTM,CNORM,
     5  NFTSCF,  IERR)
      IF(IERR.NE.0) WRITE(6,402)
      WRITE(6,12)
C
C.... COMPUTATION FINISHED
      STOP
C
C.... ERROR STOP
  400 WRITE(6,402)
  402 FORMAT(///19X,66('*')//'ERROR - COMPUTATION TERMINATED DUE TO ',
     1  'SOME ERROR(S)'//19X,66('*'))
      STOP
C
      END

      !***********************************************************************

      SUBROUTINE INTIN(TITLE,  ZNUC,RNUC,ALPHA,NUCMDL,  C,
     1  NSYM,NBS,NPBS,ZETA,CBS,N1,N5,  NTERM,NQNTM,  MTDPMX,
     2  NMX,  LOCMX,LOCTR,LOCXP,  COEFB,NUMIN,NUMAX)
C
      IMPLICIT REAL*8 (A-H,O-Z)
      INTEGER*4 P
      CHARACTER*8 TITLE(20)
      DIMENSION NBS(7),NPBS(N1,*),ZETA(N5,N1,*),CBS(N5,N1,2,*)
      DIMENSION NTERM(2,7),NQNTM(2,2,7)
      DIMENSION LOCMX(*),LOCTR(*),LOCXP(*)
      DIMENSION COEFB(0:6,7,7),NUMIN(7,7),NUMAX(7,7)
      CHARACTER*4 LABEL(7)
      DIMENSION NQNTML(7)
      DATA LABEL/'S+  ','P-  ','P+  ','D-  ', 'D+  ','F-  ','F+  '/
      DATA NQNTML/1,2,2,3,3,4,4/
C          N-QUANTUM NUMBERS OF LARGE COMP BASIS SPINORS
      DATA CSPEED/137.03599976D0/
C
C.... READ INPUT DATA
      WRITE(6,20)
   20 FORMAT(//'INPUT DATA  ',67('-'))
C=====================================================================
      READ(5,22) (TITLE(I),I=1,20)
   22 FORMAT(10A8)
      READ(5, *) ZNUC,NUCMDL,RNUC,ALPHA
C       NUCMDL = 1; POINT NUCLEUS
C              = 2; FINITE SPHERE NUCLEUS
C              = 3; GAUSSIAN NUCLEUS
C
      READ(5, *) NSYM,MTDPMX,C
C       MTDPMX = 1; P-SUPERMATRIX TO BE STORED IN CORE MEMORIES
C              = 2; P-SUPERMATRIX TO BE STORED IN DISK FILE
C
      IF(NUCMDL.EQ.0) NUCMDL=2
      IF(C.EQ.0.0D0) C=CSPEED
      IF(MTDPMX.EQ.0) MTDPMX=1
C
      DO 28 L=1,NSYM
      READ(5,*) NBS(L),(NPBS(LP,L),LP=1,NBS(L))
      DO 24 LP=1,NBS(L)
      READ(5, *) (ZETA(P,LP,L),CBS(P,LP,1,L),
     1        CBS(P,LP,2,L),P=1,NPBS(LP,L))
C       READ ZERO COEFFICIENTS FOR SMALL COMP BASIS SPINORS
C       IF THEY ARE TO BE GENERATED BY THE KINETIC BALANCE SCHEME
   24 CONTINUE
   28 CONTINUE
C======================================================================
C.... PRINT INPUT DATA
      WRITE(6,30) (TITLE(I),I=1,20)
   30 FORMAT(/10A8/10A8)
C
      WRITE(6,31) ZNUC
   31 FORMAT(/'NUCLEAR CHARGE',F12.5)
C
      IF(NUCMDL.EQ.1) WRITE(6,33)
      IF(NUCMDL.EQ.2) WRITE(6,35) RNUC
      IF(NUCMDL.EQ.3) WRITE(6,37) ALPHA
   33 FORMAT(/'NUCLEUS MODEL: POINT NUCLEUS')
   35 FORMAT(/'NUCLEUS MODEL: FINITE SPHERE NUCLEUS'/
     1     '  NUCLEAR RADIUS',D22.12)
   37 FORMAT(/'NUCLEUS MODEL: GAUSSIAN NUCLEUS'/
     1     '  PARAMETER ALPHA',D22.12)
C
      WRITE(6,42) C
   42 FORMAT(/'SPEED OF LIGHT',D22.12)
C
      IF(MTDPMX.EQ.1) WRITE(6,46)
      IF(MTDPMX.EQ.2) WRITE(6,47)
   46 FORMAT(/'P-SUPERMATRIX: STORED IN FAST MEMORIES')
   47 FORMAT(/'P-SUPERMATRIX: STORED IN DISK FILE')
C
      WRITE(6,32) NSYM
   32 FORMAT(/'NUMBER OF SYMMETRY SPECIES',I5)
      WRITE(6,34)
   34 FORMAT(/'SYMMETRY  BASIS         EXPONENT PARAM',8X,
     1  'LARGE-COMP COEFF',8X,'SMALL-COMP COEFF')
      DO 36 L=1,NSYM
      WRITE(6,41) LABEL(L),(ZETA(P,1,L),CBS(P,1,1,L),
     1        CBS(P,1,2,L),P=1,NPBS(1,L))
   41 FORMAT(2X,A4,7X,'1',1p,3D24.14,:/(14X,3D24.14))
      DO 38 LP=2,NBS(L)
      WRITE(6,39) LP,(ZETA(P,LP,L),CBS(P,LP,1,L),
     1        CBS(P,LP,2,L),P=1,NPBS(LP,L))
   39 FORMAT(10X,I4,1p,3D24.14,:/(14X,3D24.14))
   38 CONTINUE
   36 CONTINUE
C
C
C.... CALC PARAMETERS
      NMX=0
      NXP=0
      NBSMAX=0
      DO 50 L=1,NSYM
      LOCMX(L)=NMX
      LOCXP(L)=NXP
      NMX=NMX+NBS(L)*(NBS(L)*2+1)
      NXP=NXP+(NBS(L)*(NBS(L)+1))/2
      IF(NBSMAX.LT.NBS(L)) NBSMAX=NBS(L)
   50 CONTINUE
C
      LOCTR(1)=0
      DO  N=1,NBSMAX*2-1
        LOCTR(N+1)=LOCTR(N)+N
      ENDDO
C
      CALL BCOEF( COEFB, NUMIN, NUMAX )
C
C     NUMBER OF TERMS AND QUANTUM NUMBERS OF BASIS SPINORS
      ! LARGE COMPONENT
      DO L = 1, NSYM
        NTERM(1,L) = 1
        NQNTM(1,1,L) = NQNTML(L)
      ENDDO
      ! SMALL COMPONENT
      DO L=1,NSYM,2
        NTERM(2,L)=1
        NQNTM(1,2,L)=NQNTML(L)+1
      ENDDO 
      DO L=2,NSYM,2
        NTERM(2,L)=2
        NQNTM(1,2,L)=NQNTML(L)-1
        NQNTM(2,2,L)=NQNTML(L)+1
      ENDDO  
      
cAV
      do L = 1, NSYM
        write(*,*) ' '
        do i = 1, 2
          do j = 1, 2
            write(*,88) i, j, L, NQNTM(i,j,l)
88          format(' NQNTM(',i2,',',i2,',',i2,') = ',i4)
          enddo
        enddo
      enddo
c      stop 
cendAV
      
C
C     GENERATE SMALL-COMPONENT COEFFICIENTS OF BASIS SPINORS
C     ACCORDING TO KINETIC ENERGY BALANCE SCHEME
      DO L=1,NSYM
       DO LP=1,NBS(L)
        DO P=1,NPBS(LP,L)
          IF (CBS(P,LP,2,L).EQ.0.0D0) THEN
            CBS(P,LP,2,L)=CBS(P,LP,1,L) * SQRT(ZETA(P,LP,L))
          ENDIF
        ENDDO
       ENDDO
      ENDDO
C
      END

      !***********************************************************************

      SUBROUTINE BCOEF(B,NUMIN,NUMAX)
C
      IMPLICIT REAL*8 (A-H,O-Z)
      INTEGER*4 DENOM
      DIMENSION B(0:6,7,7),NUMIN(7,7),NUMAX(7,7)
      DIMENSION J2(7),LQNTM(7),NUMTR(0:6,10),DENOM(0:6,10)
      DATA J2/1,1,3,3,5,5,7/,LQNTM/0,1,1,2,2,3,3/
      DATA NUMTR/1,1,5*0, 0,1,1,4*0, 3*1,9,3*0, 0,0,1,1,3*0,
     1  0,1,1,2,2,0,0, 1,1,8,8,2,50,0, 3*0,1,1,0,0,
     2  0,0,9,1,5,5,0, 0,3*1,5,5,25, 1,1,5,3,9,75,25/
      DATA DENOM/1,3,5*0, 0,3,5,4*0, 2,30,10,70,3*0, 0,0,5,7,3*0,
     1  0,5,35,35,21,0,0, 3,105,105,315,63,693,0, 3*0,7,9,0,0,
     2  0,0,70,42,126,66,0, 0,7,105,21,231,231,429,
     3  4,252,84,308,308,4004,1716/
C        (J,J')=(1/2,1/2),(3/2,1/2),(3/2,3/2),(5/2,1/2), ...
C
C.... COMPUTE B(NU)-COEFF ACCORDING TO GRANT'S TABLE 5
C          B-COEFF ARE DEFINED BY EQ (A19) IN Y. KIM: PHYS. REV.
C          154 (1967) 17 AND HALVES OF GAMMA IN I. P. GRANT:
C          PROC. ROY. SOC. A262 (1961) 555
C          THERE ARE TWO ERRORS IN THE TABLE;
C          CORRECTED VALUES ARE
C          GAMMA(7/2,1,7/2)=1/252 AND G(7/2,3,7/2)=3/308
C
      DO L=1,7
       L1=(L-1)/2+1
       DO M=1,L
        M1=(M-1)/2+1
C
        MINNU=IABS(J2(L)-J2(M))/2
        MAXNU=(J2(L)+J2(M))/2
        LM=LQNTM(L)+LQNTM(M)
        IF(((LM/2)*2.EQ.LM).AND.((MINNU/2)*2.NE.MINNU))
     1    MINNU=MINNU+1
        IF(((LM/2)*2.NE.LM).AND.((MINNU/2)*2.EQ.MINNU))
     1    MINNU=MINNU+1
        IF(((MINNU+MAXNU)/2)*2.NE.(MINNU+MAXNU)) MAXNU=MAXNU-1
C
        NUMIN(L,M)=MINNU
        NUMIN(M,L)=MINNU
        NUMAX(L,M)=MAXNU
        NUMAX(M,L)=MAXNU
C
        LM=(L1*(L1-1))/2+M1
        DO NU=MINNU,MAXNU,2
          B(NU,M,L)=0.5D0*DBLE(NUMTR(NU,LM))/DBLE(DENOM(NU,LM))
          B(NU,L,M)=B(NU,M,L)
        ENDDO 
       ENDDO  
      ENDDO
C
      END

      !***********************************************************************

      SUBROUTINE SCFIN(NSYM,NOS,  NCONF,WAV,OCUPAV,
     1  MXITR,THCVL,THCVS,THCVEN,THDLL,THDSL,THDSS,
     2  IXTRP,DFCTR,  IPRVC,IPRMX,INTLVC,  VC,  NFT,
     3  OCUP,VCC,  NBS,  NVC,LOCVC)
C
      IMPLICIT REAL*8 (A-H,O-Z)
      DIMENSION NOS(3,*)
      DIMENSION WAV(*),OCUPAV(7,*)
      DIMENSION VC(*)
      DIMENSION OCUP(2,*),VCC(*)
      DIMENSION NBS(*)
      DIMENSION LOCVC(*)
      dimension DUMM(1)
      CHARACTER*4 LABEL(7)
      DATA LABEL/'S+  ','P-  ','P+  ','D-  ','D+  ','F-  ','F+  '/
      DATA THCVL0,THCVS0/1.0D-6,1.0D-8/
      DATA THDLL0,THDSL0,THDSS0/1.0D-5,1.0D-7,1.0D-9/
C
C.... READ INPUT DATA (PART 1)
C=====================================================================
      READ(5, *) (NOS(1,L),L=1,NSYM)
      READ(5, *) (NOS(2,L),L=1,NSYM)
      READ(5, *) NCONF
      IF(NCONF.NE.0) THEN
        DO I=1,NCONF
          READ(5, *) (OCUPAV(L,I),L=1,NSYM)
        ENDDO
      END IF
C=====================================================================
      WRITE(6,36) (LABEL(L),L=1,NSYM)
   36 FORMAT(/27X,10(4X,A2))
      WRITE(6,42) (NOS(1,L),L=1,NSYM)
   42 FORMAT('NUMBER OF CLOSED SHELLS',4X,10I6)
      WRITE(6,43) (NOS(2,L),L=1,NSYM)
   43 FORMAT(10X,'OPEN SHELLS',6X,10I6)
C
      WRITE(6,44) NCONF
   44 FORMAT(/'NUMBER OF OPEN SHELL CONFG',I7)
      IF(NCONF.NE.0) THEN
        WRITE(6,46)
   46   FORMAT(/'CONF',17X,'OPEN SHELL OCCUPATION NUMBERS')
        DO I=1,NCONF
          WRITE(6,50) I,(OCUPAV(L,I),L=1,NSYM)
   50     FORMAT(I3,24X,20F6.1)
        ENDDO
      END IF
C======================================================================
      READ(5, *) MXITR, IXTRP, DFCTR
C          IXTRP =  0;  NO EXTRAPOLATION
C                =  1;  HARTREE DAMPING OF SCF VECTORS
C                =  2;  HARTREE DAMPING OF FOCK MATRICES
C                       (NEW C 0R F):= (1-DFCTR)*(NEW) + DFCTR*(OLD)
C                =  3;  Aitken extrapolation of SCF VECTORS
C                =  4;  Aitken extrapolation of Fock matrices 
c                = -n;  first n cycles - damping, then - extrapolation
      READ(5, *) THCVL,THCVS,THCVEN,THDLL,THDSL,THDSS
      READ(5, *) IPRVC,IPRMX,INTLVC,NFT
C          IPRVC = 1  PRINT FINAL VECTORS (OCCUP)
C                = 2                      (OCCUP & VIRTUAL)
C          IPRMX = 0  NO PRINT
C                = 1  PRINT FINAL S-, H-, D-, AND F-MATRICES
C          INTLVC= 1  GUESS INITIAL C-VECTORS BY DIAG OF H-MATRIX
C                = 2  READ INITIAL C-VECTORS
C
      IF( IPRVC == 0 ) IPRVC = 1
      IF( INTLVC == 0 ) INTLVC = 1
C======================================================================
      WRITE(6,62) MXITR
   62 FORMAT(/'MAX NUMBER OF SCF ITERATIONS',I5)
C
      IF( IXTRP == 0 ) WRITE(6,660)
      IF( IXTRP == 1 ) WRITE(6,661) DFCTR
      IF( IXTRP == 2 ) WRITE(6,662) DFCTR
      IF( IXTRP == 3 ) WRITE(6,663) 
      IF( IXTRP == 4 ) WRITE(6,664) 
      IF( IXTRP  < 0 ) WRITE(6,665) ABS(IXTRP)
  660 FORMAT(/'EXTRAPOLATION: NO EXTRAP FOR C-VECTORS AND F-MATRICES')
  661 FORMAT(/'EXTRAPOLATION: HARTREE DAMPING FOR C-VECTORS'/
     1  'DAMPING FACTOR FOR OLD C-VECTORS =',F8.4)
  662 FORMAT(/'EXTRAPOLATION: HARTREE DAMPING FOR F-MATRICES'/
     1  'DAMPING FACTOR FOR OLD F-MATRICES =',F8.4)
  663 FORMAT(/'AITKEN EXTRAPOLATION FOR C-VECTORS')
  664 FORMAT(/'AITKEN EXTRAPOLATION FOR F-MATRICES')
  665 FORMAT(/'FIRST ',i3,' CYCLES - DAMPING, AFTER - EXTRAPOLATION ')
C
      IF(THCVL.EQ.0.0D0) THCVL=THCVL0
      IF(THCVS.EQ.0.0D0) THCVS=THCVS0
      IF(THDLL.EQ.0.0D0) THDLL=THDLL0
      IF(THDSL.EQ.0.0D0) THDSL=THDSL0
      IF(THDSS.EQ.0.0D0) THDSS=THDSS0
C
      WRITE(6,64) THCVL,THCVS,THCVEN,THDLL,THDSL,THDSS
   64 FORMAT(/'THRESHOLD CONVERGENCE PARAMETERS :',/,1p,
     1  5x,'COEF(LARG)',D12.3,/,5X,'COEF(SMAL)',D12.3,/,
     2  5X,'TOTL ENRGY',D12.3,/,5X,'D(LL)-MTRX',D12.3,/,
     3  5X,'D(SL)-MTRX',D12.3,/,5X,'D(SS)-MTRX',D12.3)
C
      IF(IPRVC.EQ.1) WRITE(6,681)
      IF(IPRVC.EQ.2) WRITE(6,682)
  681 FORMAT(/'PRINT C-VECTORS: FINAL OCCUPIED VECTORS')
  682 FORMAT(/'PRINT C-VECTORS: FINAL OCCUPIED AND VIRTUAL VECTORS')
      IF(IPRMX.EQ.0) WRITE(6,683)
      IF(IPRMX.EQ.1) WRITE(6,684)
  683 FORMAT(/'PRINT MATRICES: NO PRINT')
  684 FORMAT(/'PRINT MATRICES: FINAL S-, H-, D-, AND F-MATRICES')
C
      IF(INTLVC.EQ.1) WRITE(6,671)
      IF(INTLVC.EQ.2) WRITE(6,672)
  671 FORMAT(/'GUESS OF INITIAL C-VECTROS: DIAG OF H-MATRIX')
  672 FORMAT(/'INITIAL C-VECTROS: READ AS INPUT DATA')
C
      WRITE(6,69) NFT
   69 FORMAT(/'SAVE FILE NO OF SCF RESULTS',I6)
C
C.... CALC PARAMETERS
      NVC=0
      DO L=1,NSYM
        NOS(3,L)=NOS(1,L)+NOS(2,L)
        LOCVC(L)=NVC
        NBST=NBS(L)*2
        IF(NOS(3,L).NE.0) NVC=NVC+NBST*NBST
      ENDDO
C
C.... READ INPUT DATA (PART 2)
C=====================================================================
      IF(INTLVC.EQ.2) THEN
      DO L=1,NSYM
        IF(NOS(3,L).EQ.0) GO TO 80
        NMAX=LOCVC(L)
        DO I=1,NOS(3,L)
          NMIN=NMAX+1
          NMAX=NMAX+NBS(L)*2
          READ(5, *) (VC(N),N=NMIN,NMAX)
        ENDDO
   80   CONTINUE
      ENDDO
C=======================================================================
      WRITE(6,86)
   86 FORMAT(/'INITIAL C-VECTORS')
      CALL PRTVC( DUMM, VC, 1,  NSYM,NBS,NOS,LOCVC,5)
      END IF
C
C.... SET OCCUPATION NUMBERS AND CALC VECTOR COUPLING COEFFICIENTS
      CALL SETVC(OCUP,VCC,  NSYM,NOS,  NCONF,WAV,OCUPAV)
      WRITE(6,10)
   10 FORMAT(///'SYM   OPEN SHELL OCUP NO (COMPUTED)')
      WRITE(6,12) (LABEL(L),OCUP(2,L),L=1,NSYM)
   12 FORMAT(A2,F24.16)
      IF(NCONF.NE.0) THEN
      WRITE(6,14)
   14 FORMAT(//'CONF   WEIGHTS FOR CONFIGURATIONS (COMPUTED)')
      WRITE(6,16) (I,WAV(I),I=1,NCONF)
   16 FORMAT(I3,1p,D27.15)
      END IF
C
      END

      !***********************************************************************

      SUBROUTINE SPACE(N1,N2,N4,N5,N6,N7,N8,
     1  NSYM,NBS,NPBS,NOS,NCONF,  MTDPMX,  *)
C
      DIMENSION NBS(*),NPBS(N1,*),NOS(3,*)
C
C.... CHECK GIVEN STORAGE SPACES AND ISSUE ERROR MESSAGES
      M1=0
      M2=0
      M4=0
      M5=0
      M6=0
      MX=0
      DO L=1,NSYM
        IF(M1.LT.NBS(L)) M1=NBS(L)
        NBST=NBS(L)*2
        M2=M2+NBST*(NBST+1)/2
        M4=M4+NBST*NBST
        DO LP=1,NBS(L)
          IF(M5.LT.NPBS(LP,L)) M5=NPBS(LP,L)
        ENDDO
        IF(M6.LT.NOS(3,L)) M6=NOS(3,L)
        MX=MX+NBS(L)*(NBS(L)+1)/2
      ENDDO
      M7=MX*(MX+1)/2
      IF(MTDPMX.EQ.2) M7=0
C
      WRITE(6,20) M1,N1, M2,N2, M4,N4, M5,N5, M6,N6, M7,N7,
     1  NCONF,N8
   20 FORMAT(/'STORAGE SPACES REQUIRED AND GIVEN'/
     1  'N1=',2I7,'   N2=',2I7,'   N4=',2I9/
     2  'N5=',2I7,'   N6=',2I7,'   N7=',2I9,'   N8=',2I7)
C
      IF((M1.GT.N1).OR.(M2.GT.N2).OR.(M4.GT.N4).OR.
     1   (M5.GT.N5).OR.(M6.GT.N6).OR.
     2   (NCONF.GT.N8)) GO TO 30
      IF((MTDPMX.EQ.1).AND.(M7.GT.N7)) GO TO 30
C
      RETURN
C
   30 WRITE(6,32)
   32 FORMAT(/'ERROR - INSUFFICIENT STORAGE SPACE  (SUBR SPACE)')
      RETURN 1
C
      END

      !***********************************************************************

      SUBROUTINE SETVC(OCUP,VCC,  NSYM,NOS,  NCONF,WAV,OCUPAV)
C
      IMPLICIT REAL*8 (A-H,O-Z)
      DIMENSION OCUP(2,*),VCC(7,7)
      DIMENSION NOS(3,*),WAV(*),OCUPAV(7,*)
      DIMENSION OCUPC(7)
      DATA OCUPC/2*2.0D0,2*4.0D0,2*6.0D0,8.0D0/
C
C.... SET OCCUP NUMBERS OF CLOSED AND OPEN SHELLS
      IF(NCONF.NE.0) THEN
        SUM=0.0D0
        DO I=1,NCONF
C
C         CALC THE WEIGHTS OF CONFIGURATIONS ACCORDING TO
C         S.OKADA AND O.MATSUOKA: J.CHEM.PHYS. 91 (1989) 4193, APPENDIX
          W=1.0D0
          DO L=1,NSYM
cAV
c            NOCUP=OCUPC(L) + 0.0001D0
c            LOCUP=OCUPAV(L,I) + 0.0001D0
            NOCUP = nint( OCUPC(L) )
            LOCUP = nint( OCUPAV(L,I) )
            delta_N =  OCUPC(L) - dble( NOCUP )
            if( abs(delta_N) > 1.d-7 ) then
               write(*,*) ' L, OCUPC(L), NOCUP, delta = ',
     &                    L, OCUPC(L), NOCUP, delta_N
               stop
            endif  
            delta_L = OCUPAV(L,I) - dble( LOCUP )
            if( abs(delta_N) > 1.d-7 ) then
               write(*,*) ' L, I, OCUPAV(L,I), NOCUP, delta = ',
     &                    L, I, OCUPAV(L,I), LOCUP, delta_L
               stop
            endif
cendAV
            W = W * BINOM(NOCUP,LOCUP)
          ENDDO
          WAV(I)=W
C
          SUM=SUM+WAV(I)
        ENDDO
        DO I=1,NCONF
          WAV(I)=WAV(I)/SUM
        ENDDO
      END IF
C
      DO L=1,NSYM
        NOS(3,L)=NOS(1,L)+NOS(2,L)
        OCUP(1,L)=0.0D0
        OCUP(2,L)=0.0D0
        IF(NOS(1,L).NE.0) OCUP(1,L)=OCUPC(L)
        IF(NCONF.EQ.0) GO TO 10
        SUM=0.0D0
        DO I=1,NCONF
          SUM=SUM+WAV(I)*OCUPAV(L,I)
        ENDDO
        OCUP(2,L)=SUM
   10   CONTINUE
      ENDDO
C
C.... CALC VECTOR COUPLING COEFF
      DO L=1,NSYM
       DO M=1,NSYM
        VCC(L,M)=0.0D0
       ENDDO
      ENDDO
      IF(NCONF.NE.0) THEN
       DO L=1,NSYM
        DO M=1,L
         IF(NOS(2,L)*NOS(2,M).NE.0) THEN
          IF(L.EQ.M) THEN
           SUM=0.0D0
           DO I=1,NCONF
            SUM=SUM+WAV(I)*OCUPAV(L,I)*(OCUPAV(L,I)-1.0D0)
           ENDDO
           SUM=SUM*OCUPC(L)/(OCUPC(L)-1.0D0)
          ELSE
           SUM=0.0D0
           DO I=1,NCONF
            SUM=SUM+WAV(I)*OCUPAV(L,I)*OCUPAV(M,I)
           ENDDO
          END IF
         SUM=1.0D0-SUM/(OCUP(2,L)*OCUP(2,M))
         VCC(L,M)=SUM
         VCC(M,L)=SUM
         END IF   
        ENDDO
       ENDDO
      END IF
C
      END

      !***********************************************************************

      REAL*8 FUNCTION BINOM (I,J)
C
      REAL*8 FCTRL(0:8)
      DATA FCTRL/2*1.D0,2.D0,6.D0,24.D0,120.D0,720.D0,5040.D0,40320.D0/
C
C.... COMPUTE BINOMIAL COEFFICIENT (MAX(I)=8)
      BINOM=FCTRL(I)/FCTRL(J)/FCTRL(I-J)
C
      END

      !***********************************************************************

      SUBROUTINE GUESS(VC,  NSYM,NBS,NOS,
     1  LOCVC,LOCMX,  SMX,HMX,
     2  WVEC,WMX1,WMX2,WMX3,WMX4,IW,  *)
C
      IMPLICIT REAL*8 (A-H,O-Z)
      DIMENSION VC(*),SMX(*),HMX(*)
      DIMENSION NBS(*),NOS(3,*)
      DIMENSION LOCVC(*),LOCMX(*)
      DIMENSION WVEC(*),WMX1(*),WMX2(*),WMX3(*),WMX4(*),IW(*)
C
C.... CALC INITIAL SPINORS BY DIAGONALIZATION OF H-MATRIX
      DO L=1,NSYM
        IF(NOS(3,L).EQ.0) GO TO 10
        NC=LOCVC(L)+1
        MX=LOCMX(L)+1
C
        CALL GUESS1(VC(NC),NBS(L)*2,  SMX(MX),HMX(MX),
     1    WVEC,WMX1,WMX2,WMX3,WMX4,IW)
C
   10   CONTINUE
      ENDDO
C
C.... ORTHONORMALIZE OCCUP SPINORS
      CALL NORMC(VC,SMX, NSYM,NBS,NOS,LOCVC,LOCMX, WVEC,1, *100)
C
      RETURN
C
C.... PROCESS ERROR
  100 WRITE(6,102)
  102 FORMAT(/'ERROR - ERROR DETECTED WHILE CALLING SUBR REFERRED ',
     1  'ABOVE  (SUBR GUESS)')
      RETURN 1
C
      END

      !***********************************************************************

      SUBROUTINE GUESS1(VC,NBST,  SMX,HMX,  EV,A,B,V,W,IW)
C
      IMPLICIT REAL*8 (A-H,O-Z)
      INTEGER*4 P,Q,PQ
      DIMENSION VC(NBST,*),SMX(*),HMX(*)
      DIMENSION EV(*),A(NBST,*),B(NBST,*),V(NBST,*),W(NBST,*),IW(*)
      DATA EPS/1.0D-12/
C
C.... DIAGONALIZE H-MATRIX
      PQ=0
      DO P=1,NBST
       DO Q=1,P
        PQ=PQ+1
        A(P,Q)=HMX(PQ)
        A(Q,P)=HMX(PQ)
        B(P,Q)=SMX(PQ)
        B(Q,P)=SMX(PQ)
       ENDDO
      ENDDO
      CALL GEIG(A,B,EV,V,NBST,NBST,EPS,W,IW,NBST)
C
C.... REARRANGE C-VECTORS
      NBS=NBST/2
      J=NBS
      DO I=1,NBS
       J=J+1
       DO P=1,NBST
        VC(P,I)=V(P,J)
       ENDDO
      ENDDO
      J=0
      DO I=NBS+1,NBST
        J=J+1
        DO P=1,NBST
         VC(P,I)=V(P,J)
        ENDDO
      ENDDO
C
      END

      !***********************************************************************

      SUBROUTINE BSNORM(CNORM,  NSYM,NBS,NPBS,ZETA,N1,N5)
C
      IMPLICIT REAL*8 (A-H,O-Z)
      INTEGER*4 P
      DIMENSION CNORM(2,N5,N1,2,*)
      DIMENSION NBS(*),NPBS(N1,*),ZETA(N5,N1,*)
      DIMENSION V(0:10)
      DIMENSION NQNTML(7),BUNSI(7),BUMBO(7)
      DATA NQNTML/1,2,2,3,3,4,4/,
     1  BUNSI/0.D0, 6.D0,0.D0,10.D0,0.D0,14.D0,0.D0/,
     2  BUMBO/0.D0,15.D0,0.D0,35.D0,0.D0,63.D0,0.D0/
C
C.... LOOP OVER SYMMETRY SPECIES AND BASIS SPINORS
      DO L=1,NSYM
       DO LP=1,NBS(L)
        DO P=1,NPBS(LP,L)
C
C....    COMPUTE V-FACTOR
         N2 = NQNTML(L)*2
         CALL AUXV( N2+2, ZETA(P,LP,L), V )
C
C....    COMPUTE NORMALIZATION FACTORS OF PRIMITIVE BASIS SPINORS
C        LARGE COMPONENT
         CNORM(1,P,LP,1,L) = 1.0D0 / SQRT( V(N2) )
C
C        SMALL COMPONENT
         IF( MOD(L,2).EQ.0 ) THEN
           CNORM(1,P,LP,2,L) = BUNSI(L) / SQRT( BUMBO(L)*V(N2-2) )
           CNORM(2,P,LP,2,L) = -1.0D0 / SQRT( V(N2+2) )
         ELSE
           CNORM(1,P,LP,2,L) = -1.0D0 / SQRT( V(N2+2) )
         END IF
        ENDDO
       ENDDO   
      ENDDO
C
      END

      !***********************************************************************

      SUBROUTINE NORMBS(CBS,N1,N5,  NSYM,NBS,NPBS,ZETA,
     1  NTERM,NQNTM,CNORM)
C
      IMPLICIT REAL*8 (A-H,O-Z)
      DIMENSION CBS(N5,N1,2,*)
      DIMENSION NBS(*),NPBS(N1,*),ZETA(N5,N1,*)
      DIMENSION NTERM(2,7),NQNTM(2,2,7),CNORM(2,N5,N1,2,*)
      DIMENSION XS(2),V(0:10)
      CHARACTER*4 LABEL(7)
      DATA LABEL/'S+  ','P-  ','P+  ','D-  ','D+  ','F-  ','F+  '/

C.... LOOP OVER SYMMETRY SPECIES AND BASIS SPINORS
      DO L=1,NSYM
       DO LP=1,NBS(L)

C....    NORMALIZE BASIS SPINORS
         XS(1) = 0.0D0
         XS(2) = 0.0D0
         IMAX = NQNTM( NTERM(2,L), 2 ,L ) * 2
         DO IP = 1, NPBS(LP,L)
          DO JP = 1, NPBS(LP,L)
           ZAV = ( ZETA(IP,LP,L) + ZETA(JP,LP,L) ) * 0.5D0
c           write(*,*) ' ZAV. V = ',ZAV,V           
           CALL AUXV( IMAX, ZAV, V )
           DO LS=1,2
            XS(LS) = XS(LS) + 
     &               XINTS( V, NTERM(LS,L), NQNTM(1,LS,L), 
     &                      CNORM(1,IP,LP,LS,L), CNORM(1,JP,LP,LS,L) ) * 
     &               CBS(IP,LP,LS,L) * CBS(JP,LP,LS,L)
           ENDDO
          ENDDO
         ENDDO
C
         DO LS=1,2
           ANORM = 1.0D0 / DSQRT( XS(LS) )
           DO IP = 1, NPBS(LP,L)
             CBS(IP,LP,LS,L) = CBS(IP,LP,LS,L) * ANORM
           ENDDO
         ENDDO
C
       ENDDO
      ENDDO

C.... PRINT NORMALIZED BASIS SPINORS
      WRITE(6,30)
   30 FORMAT(/'NORMALIZED BASIS SPINORS'/
     1  'SYMMETRY  BASIS         EXPONENT PARAM',
     2  8X,'LARGE-COMP COEFF',8X,'SMALL-COMP COEFF')
      DO L=1,NSYM
        WRITE(6,34) LABEL(L),(ZETA(IP,1,L),CBS(IP,1,1,L),CBS(IP,1,2,L),
     1    IP=1,NPBS(1,L))
   34   FORMAT(2X,A4,7X,'1',1p,3D24.14,:/(14X,3D24.14))
        DO LP=2,NBS(L)
          WRITE(6,38) LP,(ZETA(IP,LP,L),CBS(IP,LP,1,L),CBS(IP,LP,2,L),
     1      IP=1,NPBS(LP,L))
   38     FORMAT(10X,I4,1p,3D24.14,:/(14X,3D24.14))
        ENDDO
      ENDDO
C
      END

      !***********************************************************************

      SUBROUTINE EINT1(SMX,HMX,
     1  NSYM,NBS,NPBS,ZETA,CBS,N1,N5,
     2  NTERM,NQNTM,CNORM,  ZNUC,RNUC,ALPHA,NUCMDL,  C,  LOCMX,LOCTR)
C
      IMPLICIT REAL*8 (A-H,O-Z)
      INTEGER*4 P,Q
      DIMENSION SMX(*),HMX(*)
      DIMENSION NBS(*),NPBS(N1,*),ZETA(N5,N1,*),CBS(N5,N1,2,*)
      DIMENSION NTERM(2,7),NQNTM(2,2,7),CNORM(2,N5,N1,2,*)
      DIMENSION LOCMX(*),LOCTR(*)
      DIMENSION V(0:10),W(0:4),C1(9,2:20),C2(9,2:20),XS(2),XU(2)
      COMMON /FNUT/F(0:8),TABF(0:14,0:120)
      DIMENSION SNFCTR(7)
      DATA SNFCTR/3.0D0,2*5.0D0,2*7.0D0,2*9.0D0/
C
      CC2=C*C*2.0D0
C
C.... LOOP OVER SYMMETRY SPECIES AND BASIS SPINORS
        DO L=1,NSYM
       DO LP=1,NBS(L)
      DO LQ=1,LP
C
C.... COMPUTE ONE-ELECTRON INTEGRALS OVER BASIS SPINORS
      XS(1)=0.0D0
      XS(2)=0.0D0
      XU(1)=0.0D0
      XU(2)=0.0D0
      XTPQ=0.0D0
      XTQP=0.0D0
C
      DO P=1,NPBS(LP,L)
       DO Q=1,NPBS(LQ,L)
C
C....   COMPUTE AUX FUNCTIONS V, F, W, AND C
        NQMAX=NQNTM(NTERM(2,L),2,L)
        IMAX=NQMAX*2
        ZPQ=ZETA(P,LP,L)+ZETA(Q,LQ,L)
        CALL AUXV(IMAX,ZPQ*0.5D0,V)
C
C       FOR POINT NUCLEUS
        IF(NUCMDL.EQ.1) THEN
          CALL AUXV(IMAX-1,ZPQ*0.5D0,V)
C
C       FOR FINITE SPHERE NUCLEUS
        ELSE IF(NUCMDL.EQ.2) THEN
          XI=ZPQ*(RNUC**2)
          CALL AUXF(NQMAX+1,XI)
          CALL AUXW(NQMAX-1,ZPQ,XI,W)
C
C       FOR GAUSSIAN NUCLEUS
        ELSE IF(NUCMDL.EQ.3) THEN
          CALL AUXV(IMAX-1,ZPQ*0.5D0,V)
          CALL AUXC(IMAX-1,2,ZPQ/ALPHA,C1)
          CALL AUXC(1,IMAX,ALPHA/ZPQ,C2)
        END IF
C
C....   COMPUTE ONE-ELECTRON INTEGRALS
C       S(L,L), U(L,L); S(S,S), U(S,S)
        DO I=1,2
          CC=CBS(P,LP,I,L)*CBS(Q,LQ,I,L)
          XSSS=XINTS(V,
     1      NTERM(I,L),NQNTM(1,I,L),CNORM(1,P,LP,I,L),CNORM(1,Q,LQ,I,L))
          XS(I)=XS(I)+XSSS*CC
          XU(I)=XU(I)+XINTU(RNUC,ALPHA,NUCMDL,  ZPQ,V,W,C1,C2,
     1      NTERM(I,L),NQNTM(1,I,L),CNORM(1,P,LP,I,L),CNORM(1,Q,LQ,I,L))
     2      *CC
        ENDDO
C       T(S,L)
        XTPQ=XTPQ+XSSS*DSQRT(ZETA(Q,LQ,L)*SNFCTR(L))
     1           *CBS(P,LP,2,L)*CBS(Q,LQ,1,L)
        XTQP=XTQP+XSSS*DSQRT(ZETA(P,LP,L)*SNFCTR(L))
     1           *CBS(Q,LQ,2,L)*CBS(P,LP,1,L)
       ENDDO
      ENDDO
C
C.... COMPUTE S- AND H-MATRICES
      LOCML=LOCMX(L)
      NBSL=NBS(L)
      LPQLL=LOCML+LOCTR(LP)     +LQ
      LPQSL=LOCML+LOCTR(LP+NBSL)+LQ
      LQPSL=LOCML+LOCTR(LQ+NBSL)+LP
      LPQSS=LOCML+LOCTR(LP+NBSL)+(LQ+NBSL)
C
      SMX(LPQLL)=XS(1)
      SMX(LPQSL)=0.0D0
      SMX(LQPSL)=0.0D0
      SMX(LPQSS)=XS(2)
      HMX(LPQLL)=-ZNUC*XU(1)
      HMX(LPQSL)=C*XTPQ
      HMX(LQPSL)=C*XTQP
      HMX(LPQSS)=-ZNUC*XU(2)-CC2*XS(2)
C
      ENDDO
       ENDDO
        ENDDO
C
      END

      !***********************************************************************

      SUBROUTINE AUXV( IMAX, X, V )
cAV
c      IMPLICIT REAL*8 (A-H,O-Z)
c      DIMENSION V(0:*)
      use DoubleFactorials
      IMPLICIT NONE
      INTEGER*4 :: I, IMAX, IMIN
      REAL*8 :: X, V(0:*), X1, sqrtx

      sqrtx = sqrt( X )
      do I = 0, IMAX
        V(I) = dfac(I-1) / ( sqrtx**(I+1) ) 
      enddo
      return
cendAV

C.... COMPUTE V(I,X)-FUNCTIONS (I=0(OR 1),IMAX,2)
      X1 = 1.0D0 / X
      IF( MOD(IMAX,2) .EQ. 0 ) THEN
        V(0) = DSQRT(X1)
        IMIN = 2
      ELSE
        V(1) = X1
        IMIN = 3
      END IF
      DO I = IMIN, IMAX, 2
        V(I) = V(I-2) * X1 * DBLE(I-1)
      END DO  
C     V(I)=(DBL FCTRL OF (I-1))/((SQRT(X))**(I+1))
C
      END

      !***********************************************************************

      SUBROUTINE AUXF(NUMAX,T)
C
      IMPLICIT REAL*8 (A-H,O-Z)
      COMMON /FNUT/F(0:8),TABF(0:14,0:120)
      DATA CONST/0.88622 69254 52758D0/
C              = SQRT(PI)/2
C
C.... COMPUTE F(NU,T)-FUNCTIONS (NU=0,NUMAX) USING MCMARCHIE-DAVIDSON
C     ALGORITHM (J. COMP. PHYS. 26 (1978) 218)
      NMAX=NUMAX
      X   =T
C
C.... T .LT. 12
      IF(X.GE.12.0D0) GO TO 20
      I = 10.0D0*X + 0.5D0
      DELT = 0.1D0 * DBLE(I) - X
      SUM = TABF(NMAX+6,I)*DELT/6.0D0
      DO K=5,1,-1
        SUM=(SUM+TABF(NMAX+K,I))*DELT/DBLE(K)
      ENDDO
      F(NMAX)=SUM+TABF(NMAX,I)
      IF(NMAX.EQ.0) RETURN
C
      T2=X+X
      EXPT=DEXP(-X)
      DO J=NMAX-1,0,-1
        F(J)=(T2*F(J+1)+EXPT)/DBLE(J+J+1)
      ENDDO
      RETURN
C
C.... T .GE. 12 AND T .LT. 30
   20 IF(X.GE.30.0D0) GO TO 30
      IF(X.LT.15.0D0) G=((-0.3811559346D0/X+0.321180909D0)/X
     1                    -0.2473631686D0)/X+0.4999489092D0
      IF((X.GE.15.0D0).AND.(X.LT.18.0D0))
     1  G=(0.24642845D0/X-0.24249438D0)/X+0.4998436875D0
      IF((X.GE.18.0D0).AND.(X.LT.24.0D0))
     1  G=0.499093162D0-0.2152832/X
      IF(X.GE.24.0D0) G=0.490D0
C
      EXPT=DEXP(-X)
      F(0)=CONST/DSQRT(X)-EXPT*G/X
      IF(NMAX.EQ.0) RETURN
C
   24 T2=X+X
      DO J=0,NMAX-1
        F(J+1)=(DBLE(J+J+1)*F(J)-EXPT)/T2
      ENDDO
      RETURN
C
C.... T .GE. 30
   30 F(0)=CONST/DSQRT(X)
      IF(NMAX.EQ.0) RETURN
      EXPT=0.0D0
      IF(X.LT.DBLE(2*NMAX+36)) EXPT=DEXP(-X)
      GO TO 24
C
      END

      !***********************************************************************

      SUBROUTINE FTABLE
C
      IMPLICIT REAL*8 (A-H,O-Z)
      COMMON /FNUT/F(0:8),TABF(0:14,0:120)
C
C.... MAKE TABLE OF F-FUNCTIONS
c**    NMAX=4*LMAX+6
      NMAX=14
      DO 50 I=0,120
      X=0.1D0*DBLE(I)
C
      EM=DBLE(2*NMAX+1)
      T2=X+X
      TERM=1.0D0/EM
      SUM=TERM
   52 EM=EM+2.0D0
      TERM=TERM*T2/EM
      SUM=SUM+TERM
      IF(TERM.GT.1.0D-16*SUM) GO TO 52
      EXPT=DEXP(-X)
      TABF(NMAX,I)=EXPT*SUM
C
      DO J=NMAX-1,0,-1
        TABF(J,I)=(T2*TABF(J+1,I)+EXPT)/DBLE(J+J+1)
      ENDDO
   50 CONTINUE
C
      END

      !***********************************************************************

      SUBROUTINE AUXW(NMAX,ZETA,XI,W)
C
      IMPLICIT REAL*8 (A-H,O-Z)
      DIMENSION W(0:*)
C       W(N)=((FCTRL OF N)/(ZETA**N))*SUM (L=0,N) (XI**L/(FCTRL OF L))
C
C.... COMPUTE W-FUNCTIONS
      W(0)=1.0D0
      TERM=1.0D0
      DO N=1,NMAX
        TERM=TERM*XI/DBLE(N)
        W(N)=W(N-1)+TERM
      ENDDO
C
      FCTR=1.0D0
      DO N=1,NMAX
        FCTR=FCTR*DBLE(N)/ZETA
        W(N)=W(N)*FCTR
      ENDDO
C
      END

      !***********************************************************************

      REAL*8 FUNCTION XINTS( V, NTERM, NQNTM, CP, CQ )
C
      IMPLICIT REAL*8 (A-H,O-Z)
      INTEGER*4 P, Q
      DIMENSION V(0:*), NQNTM(*), CP(*), CQ(*)
C
C.... COMPUTE S-INTEGRAL OVER PRIMITIVE BASIS SPINORS
      XS = 0.0D0
      DO P = 1, NTERM
       DO Q = 1, NTERM
         NPQ = NQNTM(P) + NQNTM(Q)
         XS = XS + V(NPQ) * CP(P) * CQ(Q)
       ENDDO
      ENDDO
      XINTS = XS
C
      END

      !***********************************************************************

      REAL*8 FUNCTION XINTU (RNUC,ALPHA,NUCMDL,  ZPQ,V,W,C1,C2,
     1  NTERM,NQNTM,CP,CQ)
C
      IMPLICIT REAL*8 (A-H,O-Z)
      INTEGER*4 P,Q
      DIMENSION V(0:*),W(0:*),C1(9,2:20),C2(9,2:20)
      COMMON /FNUT/F(0:8),TABF(0:14,0:120)
      DIMENSION NQNTM(*),CP(*),CQ(*)
      DATA UFCTR/0.79788 45608 02865D0/
C               = SQRT(2/PI)
C
C.... COMPUTE U-INTEGRAL OVER PRIMITIVE BASIS SPINORS
      XU=0.0D0
C
C     POINT NUCLEUS
      IF(NUCMDL.EQ.1) THEN
      DO P=1,NTERM
       DO Q=1,NTERM
        NPQM1=NQNTM(P)+NQNTM(Q)-1
        XU=XU+V(NPQM1)*CP(P)*CQ(Q)
       ENDDO
      ENDDO
      XU=XU+XU
C
C     FINITE SPHERE NUCLEUS
      ELSE IF(NUCMDL.EQ.2) THEN
      R2=RNUC**2
      FCTR=DEXP(-ZPQ*R2)/ZPQ
      DO P=1,NTERM
       DO Q=1,NTERM
        N=(NQNTM(P)+NQNTM(Q))/2
        TERM=FCTR*W(N-1)+(R2**N)*(3.0D0*F(N)-F(N+1))
        XU=XU+(4.0D0**N)*TERM*CP(P)*CQ(Q)
       ENDDO
      ENDDO
C
C     GAUSSIAN NUCLEUS
      ELSE IF(NUCMDL.EQ.3) THEN
      FCTR=DSQRT(ALPHA*0.5D0)
      DO P=1,NTERM
       DO Q=1,NTERM
        NPQ=NQNTM(P)+NQNTM(Q)
        XU=XU+(V(NPQ-1)*C1(NPQ-1,2)+FCTR*V(NPQ)*C2(1,NPQ))*CP(P)*CQ(Q)
       ENDDO
      ENDDO
      XU=XU+XU
      END IF
C
C     MULTIPLY FACTOR
      XINTU=XU*UFCTR
C
      END

      !***********************************************************************

      SUBROUTINE EINT2(XP,  XPBUFF,NXBUFF,NFT,  MTDPMX,
     1  NSYM,NBS,NPBS,ZETA,CBS,N1,N5,
     2  NTERM,NQNTM,CNORM,  B,NUMIN,NUMAX,  LOCXP,LOCTR)
C
      IMPLICIT REAL*8 (A-H,O-Z)
      INTEGER*4 P,Q,R,S
      DIMENSION XP(8,*)
      DIMENSION XPBUFF(NXBUFF*8)
      DIMENSION NBS(*),NPBS(N1,*),ZETA(N5,N1,*),CBS(N5,N1,2,*)
      DIMENSION NTERM(2,7),NQNTM(2,2,7),CNORM(2,N5,N1,2,*)
      DIMENSION B(0:6,7,7),NUMIN(7,7),NUMAX(7,7)
      DIMENSION LOCXP(*),LOCTR(*)
      DIMENSION V1(0:20),V2(0:20),C1(9,2:20),C2(9,2:20)
      DIMENSION XJ(4),XK1(4),XK2(4)
      DIMENSION LS1(4),LS2(4)
      DATA LS1/1,1,2,2/,LS2/1,2,1,2/
      DATA XFCTR/1.59576 91216 05731D0/
C               = 2*SQRT(2/PI)
C
C.... PREPARE FOR FILE
      IF(MTDPMX.EQ.2) THEN
      OPEN (UNIT=NFT,ACCESS='SEQUENTIAL',FORM='UNFORMATTED')
      REWIND NFT
      NX=0
      NXBUF8=NXBUFF*8
      END IF
C
C.... LOOP OVER SYMMETRY SPECIES AND BASIS SPINORS
      DO 100 L=1,NSYM
      DO 101 LP=1,NBS(L)
      DO 102 LQ=1,LP
      
      DO 110 M=1,L
      MAXR=NBS(M)
      IF(L.EQ.M) MAXR=LP
      
      DO 111 MR=1,MAXR
      MAXS=MR
      IF((L.EQ.M).AND.(LP.EQ.MR)) MAXS=LQ
      
      DO 112 MS=1,MAXS

C.... COMPUTE TWO-ELECTRON J AND K INTEGRALS OVER BASIS SPINORS
      DO I=1,4
        XJ (I)=0.0D0
        XK1(I)=0.0D0
        XK2(I)=0.0D0
      ENDDO
C
      DO 120 P=1,NPBS(LP,L)
      DO 121 Q=1,NPBS(LQ,L)
      DO 122 R=1,NPBS(MR,M)
      DO 123 S=1,NPBS(MS,M)
C
C.... COMPUTE J(LPQ,MRS) INTEGRALS
      NPQ=NQNTM(NTERM(2,L),2,L)*2
      NRS=NQNTM(NTERM(2,M),2,M)*2
      ZPQ=(ZETA(P,LP,L)+ZETA(Q,LQ,L))*0.5D0
      ZRS=(ZETA(R,MR,M)+ZETA(S,MS,M))*0.5D0
      CALL AUXV1(NPQ,ZPQ,V1)
      CALL AUXV1(NRS,ZRS,V2)
      CALL AUXC(NPQ-1,NRS,ZPQ/ZRS,C1)
      CALL AUXC(NRS-1,NPQ,ZRS/ZPQ,C2)
C
C     LOOP OVER INTEGRALS J(LL,LL), J(LL,SS), J(SS,LL), J(SS,SS)
      DO 20 I=1,4
       LS1I=LS1(I)
       LS2I=LS2(I)
C
       XINT=0.0D0
       NTERM1=NTERM(LS1I,L)
       NTERM2=NTERM(LS2I,M)
       DO 22 IP=1,NTERM1
        DO IQ=1,NTERM1
         DO IR=1,NTERM2
          DO IS=1,NTERM2
           NPQ=NQNTM(IP,LS1I,L)+NQNTM(IQ,LS1I,L)
           NRS=NQNTM(IR,LS2I,M)+NQNTM(IS,LS2I,M)
           TERM=V1(NPQ-1)*V2(NRS)*C1(NPQ-1,NRS)
     1         +V2(NRS-1)*V1(NPQ)*C2(NRS-1,NPQ)
           XINT=XINT+TERM*CNORM(IP,P,LP,LS1I,L)*CNORM(IQ,Q,LQ,LS1I,L)
     1                   *CNORM(IR,R,MR,LS2I,M)*CNORM(IS,S,MS,LS2I,M)
          ENDDO
         ENDDO
        ENDDO
   22  CONTINUE
       CCCC=CBS(P,LP,LS1I,L)*CBS(Q,LQ,LS1I,L)
     1     *CBS(R,MR,LS2I,M)*CBS(S,MS,LS2I,M)
       XJ(I)=XJ(I)+XINT*XFCTR*CCCC
   20 CONTINUE
C
C.... COMPUTE K(LPQ,MRS) INTEGRALS
      MINNU=NUMIN(L,M)
      MAXNU=NUMAX(L,M)
      NPS=NQNTM(NTERM(2,L),2,L)+NQNTM(NTERM(2,M),2,M)
      NQR=NPS
      ZPS=(ZETA(P,LP,L)+ZETA(S,MS,M))*0.5D0
      ZQR=(ZETA(Q,LQ,L)+ZETA(R,MR,M))*0.5D0
      CALL AUXV1(NPS+MAXNU,ZPS,V1)
      CALL AUXV1(NQR+MAXNU,ZQR,V2)
      CALL AUXC(NPS-1,NQR+MAXNU,ZPS/ZQR,C1)
      CALL AUXC(NQR-1,NPS+MAXNU,ZQR/ZPS,C2)
C
C     LOOP OVER INTEGRALS K(LL,LL), K(LS,SL), K(SL,LS), K(SS,SS)
      DO 32 I=1,4
       LS1I=LS1(I)
       LS2I=LS2(I)
C
       XINT=0.0D0
       NTERMP=NTERM(LS1I,L)
       NTERMQ=NTERM(LS2I,L)
       NTERMR=NTERM(LS2I,M)
       NTERMS=NTERM(LS1I,M)
       DO 34 IP=1,NTERMP
        DO IQ=1,NTERMQ
         DO IR=1,NTERMR
          DO IS=1,NTERMS
           NPS=NQNTM(IP,LS1I,L)+NQNTM(IS,LS1I,M)
           NQR=NQNTM(IQ,LS2I,L)+NQNTM(IR,LS2I,M)
C
           TERM=0.0D0
           DO 36 NU=MINNU,MAXNU,2
            TERM=TERM+(V1(NPS-NU-1)*V2(NQR+NU)*C1(NPS-NU-1,NQR+NU)
     1                +V2(NQR-NU-1)*V1(NPS+NU)*C2(NQR-NU-1,NPS+NU))
     2               *B(NU,L,M)
   36      CONTINUE
C
           XINT=XINT+TERM*CNORM(IP,P,LP,LS1I,L)*CNORM(IQ,Q,LQ,LS2I,L)
     1                   *CNORM(IR,R,MR,LS2I,M)*CNORM(IS,S,MS,LS1I,M)
          ENDDO
         ENDDO
        ENDDO
   34  CONTINUE
       CCCC=CBS(P,LP,LS1I,L)*CBS(Q,LQ,LS2I,L)
     1     *CBS(R,MR,LS2I,M)*CBS(S,MS,LS1I,M)
       XK1(I)=XK1(I)+XINT*XFCTR*CCCC
   32 CONTINUE
C
C.... COMPUTE K(LPQ,MSR) INTEGRALS
      NPR=NQNTM(NTERM(2,L),2,L)+NQNTM(NTERM(2,M),2,M)
      NQS=NPR
      ZPR=(ZETA(P,LP,L)+ZETA(R,MR,M))*0.5D0
      ZQS=(ZETA(Q,LQ,L)+ZETA(S,MS,M))*0.5D0
      CALL AUXV1(NPR+MAXNU,ZPR,V1)
      CALL AUXV1(NQS+MAXNU,ZQS,V2)
      CALL AUXC(NPR-1,NQS+MAXNU,ZPR/ZQS,C1)
      CALL AUXC(NQS-1,NPR+MAXNU,ZQS/ZPR,C2)
C
C     LOOP OVER INTEGRALS K(LL,LL), K(LS,SL), K(SL,LS), K(SS,SS)
      DO 33 I=1,4
       LS1I=LS1(I)
       LS2I=LS2(I)
C
       XINT=0.0D0
       NTERMP=NTERM(LS1I,L)
       NTERMQ=NTERM(LS2I,L)
       NTERMS=NTERM(LS2I,M)
       NTERMR=NTERM(LS1I,M)
       DO 35 IP=1,NTERMP
        DO IQ=1,NTERMQ
         DO IS=1,NTERMS
          DO IR=1,NTERMR
           NPR=NQNTM(IP,LS1I,L)+NQNTM(IR,LS1I,M)
           NQS=NQNTM(IQ,LS2I,L)+NQNTM(IS,LS2I,M)
C
           TERM=0.0D0
           DO 37 NU=MINNU,MAXNU,2
             TERM=TERM+(V1(NPR-NU-1)*V2(NQS+NU)*C1(NPR-NU-1,NQS+NU)
     1                 +V2(NQS-NU-1)*V1(NPR+NU)*C2(NQS-NU-1,NPR+NU))
     2                *B(NU,L,M)
   37      CONTINUE
C
           XINT=XINT+TERM*CNORM(IP,P,LP,LS1I,L)*CNORM(IQ,Q,LQ,LS2I,L)
     1                   *CNORM(IS,S,MS,LS2I,M)*CNORM(IR,R,MR,LS1I,M)
          ENDDO
         ENDDO
        ENDDO
   35  CONTINUE
       CCCC=CBS(P,LP,LS1I,L)*CBS(Q,LQ,LS2I,L)
     1     *CBS(S,MS,LS2I,M)*CBS(R,MR,LS1I,M)
       XK2(I)=XK2(I)+XINT*XFCTR*CCCC
   33 CONTINUE
C
   
  123 CONTINUE
  122 CONTINUE
  121 CONTINUE
  120 CONTINUE
C
C.... COMPUTE P-INTEGRALS
      IF(MTDPMX.EQ.1) THEN
      LPQ=LOCXP(L)+LOCTR(LP)+LQ
      MRS=LOCXP(M)+LOCTR(MR)+MS
      LPQMRS=((LPQ-1)*LPQ)/2+MRS
C
      XP(1,LPQMRS)=XJ(1)-0.5D0*(XK1(1)+XK2(1))
      XP(2,LPQMRS)=XJ(2)
      XP(3,LPQMRS)=XJ(3)
      XP(4,LPQMRS)=-XK1(2)
      XP(5,LPQMRS)=-XK2(2)
      XP(6,LPQMRS)=-XK1(3)
      XP(7,LPQMRS)=-XK2(3)
      XP(8,LPQMRS)=XJ(4)-0.5D0*(XK1(4)+XK2(4))
C         XP(1)=P(P(L)Q(L),R(L)S(L))  (L=LARGE COMP)
C         XP(2)=P(P(L)Q(L),R(S)S(S))  (S=SMALL COMP)
C         XP(3)=P(P(S)Q(S),R(L)S(L))
C         XP(4)=P(P(L)Q(S),R(S)S(L))
C         XP(5)=P(P(L)Q(S),S(S)R(L))
C         XP(6)=P(P(S)Q(L),R(L)S(S))
C         XP(7)=P(P(S)Q(L),S(L)R(S))
C         XP(8)=P(P(S)Q(S),R(S)S(S))
C
      ELSE IF(MTDPMX.EQ.2) THEN
      XPBUFF(NX+1)=XJ(1)-0.5D0*(XK1(1)+XK2(1))
      XPBUFF(NX+2)=XJ(2)
      XPBUFF(NX+3)=XJ(3)
      XPBUFF(NX+4)=-XK1(2)
      XPBUFF(NX+5)=-XK2(2)
      XPBUFF(NX+6)=-XK1(3)
      XPBUFF(NX+7)=-XK2(3)
      XPBUFF(NX+8)=XJ(4)-0.5D0*(XK1(4)+XK2(4))
      NX=NX+8
        IF(NX.EQ.NXBUF8) THEN
      WRITE(NFT) XPBUFF
      NX=0
        END IF
      END IF
C
  112 CONTINUE
  111 CONTINUE
  110 CONTINUE
  
  
  102 CONTINUE
  101 CONTINUE
  100 CONTINUE
C
      IF((MTDPMX.EQ.2).AND.(NX.NE.0)) WRITE(NFT) XPBUFF
C
      END

      !***********************************************************************

      SUBROUTINE AUXV1(IMAX,X,V)
C
      IMPLICIT REAL*8 (A-H,O-Z)
      DIMENSION V(0:*)
C       V(I)=(DBL FCTRL OF (I-1))/((SQRT(X))**(I+1))  (IMAX.GE.1)
C
C.... COMPUTE V(I,X)-FUNCTIONS (I=0,IMAX)
      X1=1.0D0/X
      V(0)=DSQRT(X1)
      V(1)=X1
C
      DO I=2,IMAX,2
        V(I)=V(I-2)*X1*DBLE(I-1)
      ENDDO
      DO I=3,IMAX,2
        V(I)=V(I-2)*X1*DBLE(I-1)
      ENDDO
C
      END

      !***********************************************************************

      SUBROUTINE AUXC(MAXA,MAXB,T,C)
C
      IMPLICIT REAL*8 (A-H,O-Z)
      DIMENSION C(9,2:20)
C       C(A,B) A=1,3,5,...,MAXA; B=2,4,6,...,MAXB
C
C.... COMPUTE C(A,B/T)-FUNCTIONS
      T1=1.0D0/(1.0D0+T)
      T2=T1*T
C
C     STARTING ELEMENT
      C(1,2)=T1*DSQRT(T1)
C
C     (B=2) COLUMN
      IF(MAXA.GT.1) THEN
      D=C(1,2)
      DO I=3,MAXA,2
        D=D*T2*DBLE(I)/DBLE(I-1)
        C(I,2)=C(I-2,2)+D
      ENDDO
      END IF
C
C     (A=1) ROW
      DO J=4,MAXB,2
        C(1,J)=C(1,J-2)*T1
      ENDDO
C
C     A.GE.3  AND  B.GE.4
      DO J=4,MAXB,2
       DO I=3,MAXA,2
        C(I,J)=C(I,J-2)*T1+C(I-2,J)*T2
       ENDDO
      ENDDO
C
      END

      !***********************************************************************

      SUBROUTINE ITERAT(EV,VC,N3,  EMASS,EKIN,EPOT,ETOT,
     1  DIFVCL,DIFVCS,DIFFEN,DIFDLL,DIFDSL,DIFDSS,
     2  NSYM,NBS,NOS,OCUP,  VCC,  C,
     3  MXITR,THCVL,THCVS,THCVEN,THDLL,THDSL,THDSS,ICONV,
     4  IXTRP,DFCTR,  LOCVC,LOCMX,LOCTR,
     5  NMX,  SMX,HMX,  DTMX,DOMX,  PMX,QMX,  FCMX,FOMX,
     6  NVC,VCM1,  DTMXM1,FCMXM1,FOMXM1,  MTDPMX,
     7  XP,  XPBUFF,NXBUFF,NFT,
     8  G,E,V,W,WORK,IW,  ITER,  IERR,  *)
C
      IMPLICIT REAL*8 (A-H,O-Z)
      DIMENSION EV(N3,*),VC(*)
      DIMENSION NBS(*),NOS(3,*),OCUP(2,*),VCC(*)
      DIMENSION LOCVC(*),LOCMX(*),LOCTR(*)
      DIMENSION SMX(*),HMX(*),DTMX(*),DOMX(*),PMX(*),QMX(*),
     1  FCMX(*),FOMX(*)
      DIMENSION VCM1(*),DTMXM1(*),FCMXM1(*),FOMXM1(*)
      DIMENSION XP(*),G(*),E(*),V(*),W(*),WORK(*),IW(*)
      DIMENSION XPBUFF(*)
c**    EQUIVVALENCE (PMX,FCMX),(QMX,FOMX)
cAV max number of iterations that result in increase in energy
c      DATA NEUPMX/4/
      DATA NEUPMX/ 999 /
      real(8), save :: DFCTR_SAVED
cendAV 
      DFCTR_SAVED = DFCTR

C.... BEGIN SCF ITERATIONS
      ETOTM1=0.0D0
      ICONV=0
C
      ITER=0
      NEUP=0
C
      ! ----------------------------------------------------------------           
      ! IMPLEMENT SCF ITERATIONS

100   CONTINUE
      ITER = ITER + 1
       
      ! SAVE DENSITY MATRICES FROM PREVIOUS CYCLE
      DTMXM1(1:NMX) = DTMX(1:NMX)
            
      ! FORM NEW DENSITY MATRICES
      CALL FORMD(DTMX,DOMX,VC,NSYM,NBS,NOS,OCUP,NMX,LOCMX,LOCVC)

      ! QUERY CONVERGENCE ON TOTAL DENSITY MATRIX
      IF( ITER > 1 )
     1  CALL CONVD(DTMX,DTMXM1,  NSYM,NBS,NOS,  LOCMX,
     2  THDLL,THDSL,THDSS,  DIFDLL,DIFDSL,DIFDSS,  ICONV,*200)

      ! FORM P- AND Q-MARICES
      CALL FORMPQ(PMX,QMX,  DTMX,DOMX,  NSYM,NBS,NOS,VCC,
     1  NMX,LOCMX,LOCTR,  MTDPMX,  XP,XPBUFF,NXBUFF,NFT,*230)

      ! COMPUTE ENERGIES
      CALL FORME( EMASS, EKIN, EPOT, ETOT,
     1  NSYM,NBS,  C,  SMX,HMX,PMX,QMX,DTMX,DOMX,  LOCMX,LOCTR)

      ! if energy increases, increase damping factor
      if( ETOT > ETOTM1 ) DFCTR = 0.05d0 * ( 1.d0 - DFCTR ) +  DFCTR

      WRITE(6,12) ETOT, ITER
      write(6,13) ETOT - ETOTM1, DFCTR
   12 FORMAT(/'NOTE - TOTAL ENERGY =',1p,D23.15,' AT ITERATION NO',I5)
   13 FORMAT( '       DELTA ENERGY =',1p,sp,e12.4,
     &            ',  DFCTR = ',s,0p,f20.16)

      IF( ETOT > ETOTM1 ) NEUP = NEUP + 1
      IF( NEUP >= NEUPMX ) GO TO 214
      
      ! FORM FOCK MATRICES
      CALL FORMF( FCMX, FOMX, HMX, PMX, QMX, NMX )
      
      ! -----  DAMPING OF FOCK MATRICES  -----  
      IF( IXTRP == 2 .and. ITER >= 2 ) 
     &  CALL XTRPF( FCMX, FCMXM1, FOMX, FOMXM1, DFCTR, NMX )
      
      ! SAVE FOCK MATRICES FROM THIS CYCLE
      IF( IXTRP == 2 ) THEN
        FCMXM1(1:NMX) = FCMX(1:NMX)
        FOMXM1(1:NMX) = FOMX(1:NMX)
      END IF

c      ! -----  DAMPING OF FOCK MATRICES  -----  
c      IF( ABS(IXTRP) == 2 .and. ITER >= 2 ) THEN      
c        if( IXTRP < 0 .and. ITER <= 120 ) then
c          CALL XTRPF( FCMX, FCMXM1, FOMX, FOMXM1, DFCTR, NMX )
c          CALL XTRPF_HURRY( FCMX, FOMX, NMX, 0 )
c        else
c          write(*,*)
c          write(*,*) 'DOING HURRY EXTRAPOLATION OF DAMPED FOCK MATRICES'
c          CALL XTRPF( FCMX, FCMXM1, FOMX, FOMXM1, DFCTR, NMX )
c          CALL XTRPF_HURRY( FCMX, FOMX, NMX, 1 )
c        endif  
c        ! SAVE FOCK MATRICES FROM THIS CYCLE
c        FCMXM1(1:NMX) = FCMX(1:NMX)
c        FOMXM1(1:NMX) = FOMX(1:NMX)
c      ENDIF  

      ! SAVE COEFFICIENTS 
      VCM1(1:NVC) = VC(1:NVC)

      ! SOLVE EIGEN PROBLEM
      CALL EIGEN(EV,VC,N3,  NSYM,NBS,NOS,OCUP,  SMX,
     1  FCMX,FOMX,  ITER,  G,E,V,W,WORK,IW,  LOCVC,LOCMX)
      CALL NORMC(VC,SMX,NSYM,NBS,NOS,LOCVC,LOCMX,WORK,2,*230)
      CALL PHASE(VC,SMX,VCM1,  NSYM,NBS,NOS,  LOCMX,LOCVC)
      
      ! QUERY CONVERGENCE ON VECTORS AND TOTAL ENERGY
      CALL CONVC(VC,VCM1,THCVL,THCVS,DIFVCL,DIFVCS,
     1  ETOT,ETOTM1,THCVEN,DIFFEN,
     2  NSYM,NBS,NOS,LOCVC,  ICONV,*200)

      !  QUERY ITERATION NUMBER
      IF( ITER >= MXITR ) GO TO 210

      ! DAMPING APPLIED TO OCCUPIED VECTORS
      IF( IXTRP == 1 .AND. ITER >= 2 ) THEN
        CALL XTRPC( VC, VCM1, DFCTR,  NSYM, NBS, NOS, LOCVC )
        CALL NORMC( VC, SMX, NSYM, NBS, NOS, LOCVC, LOCMX, WORK, 
     &              2, *230 )
      END IF

      ! SAVE ENERGY FROM THIS CYCLE
      ETOTM1 = ETOT
      
      ! PROCESS NEXT ITERATION
      GO TO 100
      ! ----------------------------------------------------------------           
C
C.... PROCESS FOR TERMINATION
C     ITERATION CONVERGED
  200 CONTINUE
      IERR=0
      GO TO 250
C
C     ITERATION DIVERGED
  210 WRITE(6,212)
  212 FORMAT(/'ERROR - SCF ITERATION DIVERGED  (SUBR ITERAT)')
      IERR=1
      ICONV=100
      GO TO 250
C
C     TOTAL ENERGY INCREASING
  214 WRITE(6,216) ITER, NEUP, NEUPMX 
  216 FORMAT(/'ERROR - TOTAL ENERGY INCREASING AT ITER NO =',
     1  I4,'  (SUBR ITERAT :: NEUP = ',I4,', NEUPMX = ',I4,' )')
      IERR=1
      GO TO 250
C
C     OTHER ERROR DETECTED
  230 WRITE(6,232)
  232 FORMAT(/'ERROR - ERROR DETECTED WHILE CALLING SUBR REFERRED ',
     1  'ABOVE  (SUBR ITERAT)')
      IERR=1
      RETURN 1
C
  250 CONTINUE
      RETURN
C
      END

      !***********************************************************************

      SUBROUTINE FORMD(DTMX,DOMX,VC,NSYM,NBS,NOS,OCUP,NMX,LOCMX,LOCVC)
C
      IMPLICIT REAL*8 (A-H,O-Z)
      DIMENSION DTMX(*),DOMX(*),VC(*),NBS(*),NOS(3,*),OCUP(2,*),
     1  LOCMX(*),LOCVC(*)
C
C.... INITIALIZE DENSITY MATRICES
      DTMX(1:NMX) = 0.d0
      DOMX(1:NMX) = 0.d0
C
C.... LOOP OVER SYMMETRY SPECIES
      DO L=1,NSYM
C
C....   FORM CLOSED AND OPEN SHELL DENSITY MATRICES
        NBST=NBS(L)*2
        ND=LOCMX(L)+1
        NC=LOCVC(L)+1
        IF(NOS(1,L).NE.0)
     1    CALL FORMD1(DTMX(ND),VC(NC),OCUP(1,L),NBST,NOS(1,L))
        NO=NC+NOS(1,L)*NBST
        IF(NOS(2,L).NE.0)
     1    CALL FORMD1(DOMX(ND),VC(NO),OCUP(2,L),NBST,NOS(2,L))
      ENDDO
C
C.... FORM TOTAL DENSITY MATRIX
      DO N=1,NMX
        DTMX(N) = DTMX(N) + DOMX(N)
      ENDDO
C
      END

      !***********************************************************************

      SUBROUTINE FORMD1(DMX,VC,OCUP,NBST,NOS)
C
      IMPLICIT REAL*8 (A-H,O-Z)
      INTEGER*4 P,Q,PQ
      DIMENSION DMX(*),VC(NBST,*)
C
C.... LOOP OVER BASIS SPINORS
      PQ=0
      DO P=1,NBST
       DO Q=1,P
        SUM=0.0D0
        DO I=1,NOS
          SUM=SUM+VC(P,I)*VC(Q,I)
        ENDDO
        PQ=PQ+1
        DMX(PQ)=OCUP*SUM
       ENDDO
      ENDDO  
C
      END

      !***********************************************************************

      SUBROUTINE FORMPQ(PMX,QMX,  DTMX,DOMX,  NSYM,NBS,NOS,VCC,
     1  NMX,LOCMX,LOCTR,  MTDPMX,  XP,XPBUFF,NXBUFF,NFT,*)
C
      IMPLICIT REAL*8 (A-H,O-Z)
      INTEGER*4 P,Q,R,S
      DIMENSION PMX(*),QMX(*),DTMX(*),DOMX(*)
      DIMENSION NBS(*),NOS(3,*),VCC(7,7)
      DIMENSION LOCMX(*),LOCTR(*)
      DIMENSION XP(8,*),XPBUFF(NXBUFF*8)
C
C.... INITIALIZE P- AND Q-MATRICES
      DO N=1,NMX
        PMX(N)=0.0D0
        QMX(N)=0.0D0
      ENDDO
C
      IF(MTDPMX.EQ.2) THEN
      REWIND NFT
      NXBUF8=NXBUFF*8
      NX=NXBUF8
      END IF
C
C.... MULTIPLY OFF DIAG ELEMENTS OF D(LL) AND D(SS) BY 2
C     AND DIAGONAL ELEMENTS OF D(SL) BY 1/2
      LPQ=0
      DO L=1,NSYM
        NBSL=NBS(L)
        DO P=1,NBSL*2
         DO Q=1,P
          LPQ=LPQ+1
          IF(P.NE.Q) THEN
           FCTR=1.0D0
           IF((P.LE.NBSL).OR.(Q.GT.NBSL)) FCTR=2.0D0
           IF(P.EQ.(Q+NBSL)) FCTR=0.5D0
           DTMX(LPQ)=DTMX(LPQ)*FCTR
           DOMX(LPQ)=DOMX(LPQ)*FCTR
          END IF
         ENDDO
        ENDDO  
      ENDDO
C
C.... LOOP OVER SYMMETRY SPECIES AND BASIS SPINORS
      LPQMRS=0
      DO 100 L=1,NSYM
        NBSL=NBS(L)
        LOCL=LOCMX(L)
        DO 101 P=1,NBSL
         DO 102 Q=1,P
C
C         COMPUTE LOCATIONS OF MATRICES
          LPQLL=LOCL+LOCTR(P)+Q
          LPQSL=LOCL+LOCTR(P+NBSL)+Q
          LPQSS=LOCL+LOCTR(P+NBSL)+(Q+NBSL)
          LPQLS=LOCL+LOCTR(Q+NBSL)+P
C
          DO 103 M=1,L
           NBSM=NBS(M)
           LOCM=LOCMX(M)
           MAXR=NBSM
           IF(L.EQ.M) MAXR=P
           DO 104 R=1,MAXR
            MAXS=R
            IF((L.EQ.M).AND.(P.EQ.R)) MAXS=Q
            DO 105 S=1,MAXS
C
C            COMPUTE LOCATIONS OF MATRICES
             MRSLL=LOCM+LOCTR(R)+S
             MRSSL=LOCM+LOCTR(R+NBSM)+S
             MRSSS=LOCM+LOCTR(R+NBSM)+(S+NBSM)
             MRSLS=LOCM+LOCTR(S+NBSM)+R
C
             LPQMRS=LPQMRS+1
C
C....        COMPUTE P- AND Q-MATRICES
             IF(MTDPMX.EQ.1) THEN
              X1=XP(1,LPQMRS)
              X2=XP(2,LPQMRS)
              X3=XP(3,LPQMRS)
              X4=XP(4,LPQMRS)
              X5=XP(5,LPQMRS)
              X6=XP(6,LPQMRS)
              X7=XP(7,LPQMRS)
              X8=XP(8,LPQMRS)
C                  X1=P(P(L)Q(L),R(L)S(L))
C                  X2=P(P(L)Q(L),R(S)S(S))
C                  X3=P(P(S)Q(S),R(L)S(L))
C                  X4=P(P(L)Q(S),R(S)S(L))
C                  X5=P(P(L)Q(S),S(S)R(L))
C                  X6=P(P(S)Q(L),R(L)S(S))
C                  X7=P(P(S)Q(L),S(L)R(S))
C                  X8=P(P(S)Q(S),R(S)S(S))
C
             ELSE IF(MTDPMX.EQ.2) THEN
              IF(NX.EQ.NXBUF8) THEN
                READ(NFT,ERR=200,END=200) XPBUFF
                NX=0
              END IF
              X1=XPBUFF(NX+1)
              X2=XPBUFF(NX+2)
              X3=XPBUFF(NX+3)
              X4=XPBUFF(NX+4)
              X5=XPBUFF(NX+5)
              X6=XPBUFF(NX+6)
              X7=XPBUFF(NX+7)
              X8=XPBUFF(NX+8)
              NX=NX+8
             END IF
C
             IF(NOS(3,L)*NOS(3,M).EQ.0) GO TO 30
C
             PMX(LPQLL)=PMX(LPQLL)+X1*DTMX(MRSLL)+X2*DTMX(MRSSS)
             PMX(LPQSS)=PMX(LPQSS)+X8*DTMX(MRSSS)+X3*DTMX(MRSLL)
             PMX(LPQSL)=PMX(LPQSL)+X6*DTMX(MRSLS)+X7*DTMX(MRSSL)
             IF(P.NE.Q)
     1         PMX(LPQLS)=PMX(LPQLS)+X4*DTMX(MRSSL)+X5*DTMX(MRSLS)
C
             IF(LPQLL.NE.MRSLL) THEN
              PMX(MRSLL)=PMX(MRSLL)+X1*DTMX(LPQLL)+X3*DTMX(LPQSS)
              PMX(MRSSS)=PMX(MRSSS)+X8*DTMX(LPQSS)+X2*DTMX(LPQLL)
              PMX(MRSSL)=PMX(MRSSL)+X4*DTMX(LPQLS)+X7*DTMX(LPQSL)
              IF(R.NE.S)
     1          PMX(MRSLS)=PMX(MRSLS)+X6*DTMX(LPQSL)+X5*DTMX(LPQLS)
             END IF
C
             IF(NOS(2,L)*NOS(2,M).EQ.0) GO TO 30
             VCPL=VCC(L,M)
             X1=X1*VCPL
             X2=X2*VCPL
             X3=X3*VCPL
             X4=X4*VCPL
             X5=X5*VCPL
             X6=X6*VCPL
             X7=X7*VCPL
             X8=X8*VCPL
             QMX(LPQLL)=QMX(LPQLL)+X1*DOMX(MRSLL)+X2*DOMX(MRSSS)
             QMX(LPQSS)=QMX(LPQSS)+X8*DOMX(MRSSS)+X3*DOMX(MRSLL)
             QMX(LPQSL)=QMX(LPQSL)+X6*DOMX(MRSLS)+X7*DOMX(MRSSL)
             IF(P.NE.Q)
     1         QMX(LPQLS)=QMX(LPQLS)+X4*DOMX(MRSSL)+X5*DOMX(MRSLS)
C
             IF(LPQLL.NE.MRSLL) THEN
              QMX(MRSLL)=QMX(MRSLL)+X1*DOMX(LPQLL)+X3*DOMX(LPQSS)
              QMX(MRSSS)=QMX(MRSSS)+X8*DOMX(LPQSS)+X2*DOMX(LPQLL)
              QMX(MRSSL)=QMX(MRSSL)+X4*DOMX(LPQLS)+X7*DOMX(LPQSL)
              IF(R.NE.S)
     1          QMX(MRSLS)=QMX(MRSLS)+X6*DOMX(LPQSL)+X5*DOMX(LPQLS)
             END IF
C
   30        CONTINUE
C
  105       CONTINUE
  104      CONTINUE
  103     CONTINUE
  102    CONTINUE
  101   CONTINUE
  100 CONTINUE
C
C.... DELETE THE FACTOR ON DENSITY MATRICES
      LPQ=0
      DO L=1,NSYM
        NBSL=NBS(L)
        DO P=1,NBSL*2
         DO Q=1,P
          LPQ=LPQ+1
          IF(P.NE.Q) THEN
            FCTR=1.0D0
            IF((P.LE.NBSL).OR.(Q.GT.NBSL)) FCTR=0.5D0
            IF(P.EQ.(Q+NBSL)) FCTR=2.0D0
            DTMX(LPQ)=DTMX(LPQ)*FCTR
            DOMX(LPQ)=DOMX(LPQ)*FCTR
          END IF
         ENDDO
        ENDDO        
      ENDDO
C
      RETURN
C
C.... FILE ERROR DETECTED
  200 WRITE(6,202) NFT
  202 FORMAT(/'ERROR - ERROR OR END OF FILE DETECTED WHILE READING ',
     1  'DATA DET IN FILE NO',I4,'  (SUBR FORMPQ)')
C
      END

      !***********************************************************************

      SUBROUTINE FORME(EMASS,EKIN,EPOT,ETOT,
     1  NSYM,NBS,  C,  SMX,HMX,PMX,QMX,DTMX,DOMX,  LOCMX,LOCTR)
C
      IMPLICIT REAL*8 (A-H,O-Z)
      INTEGER*4 P,Q
      DIMENSION SMX(*),HMX(*),PMX(*),QMX(*),DTMX(*),DOMX(*)
      DIMENSION NBS(*),LOCMX(*),LOCTR(*)
C
C.... COMPUTE MASS ENERGY
      SUM=0.0D0
      DO L=1,NSYM
        NBSL=NBS(L)
        DO P=1,NBSL
         DO Q=1,P
           LPQ=LOCMX(L)+LOCTR(P+NBSL)+(Q+NBSL)
           TERM=SMX(LPQ)*DTMX(LPQ)
           SUM=SUM+TERM*2.0D0
         ENDDO
         SUM=SUM-TERM
        ENDDO
      ENDDO
      EMASS=-SUM*C*C*2.0D0
C
C.... COMPUTE KINETIC ENERGY
      SUM=0.0D0
      DO L=1,NSYM
        NBSL=NBS(L)
        DO P=1,NBSL
         DO Q=1,NBSL
          LPQ=LOCMX(L)+LOCTR(P+NBSL)+Q
          TERM=HMX(LPQ)*DTMX(LPQ)
          SUM=SUM+TERM
         ENDDO
        ENDDO
      ENDDO
      EKIN=SUM*2.0D0
C
C.... COMPUTE ONE- AND TWO-ELECTRON POTENTIALS, AND TOTAL ENERGIES
      SUM1=0.0D0
      SUM2=0.0D0
      LPQ=0
      DO L=1,NSYM
        NBST=NBS(L)*2
        DO P=1,NBST
         DO Q=1,P
          LPQ=LPQ+1
          TERM1=HMX(LPQ)*DTMX(LPQ)
          TERM2=PMX(LPQ)*DTMX(LPQ)-QMX(LPQ)*DOMX(LPQ)
          IF(P.NE.Q) THEN
           TERM1=TERM1+TERM1
           TERM2=TERM2+TERM2
          END IF
          SUM1=SUM1+TERM1
          SUM2=SUM2+TERM2
         ENDDO  
        ENDDO
      ENDDO
      EONE=SUM1
      ETWO=0.5D0*SUM2
      ETOT=EONE+ETWO
      EPOT=ETOT-EKIN-EMASS
C
      END

      !***********************************************************************

      SUBROUTINE FORMF(FCMX,FOMX,HMX,PMX,QMX,NMX)
C
      IMPLICIT REAL*8 (A-H,O-Z)
      DIMENSION FCMX(*),FOMX(*),HMX(*),PMX(*),QMX(*)
C     EQUIVALENCE (FCMX,PMX),(FOMX,QMX)
C
C.... FORM FOCK MATRICES
      DO N=1,NMX
        FCMX(N) = HMX(N) + PMX(N)
        FOMX(N) = FCMX(N) - QMX(N)
      ENDDO
C
      END

      !***********************************************************************
      ! HURRY exrapolation
      ! IFLAG = 0 : just calculate delta matrices, but do not alter Fock matrices
      !         1 : calculate delta matrices and modify Fock matrices     
      
      SUBROUTINE XTRPF_HURRY( FCMX, FOMX, NMX, IFLAG )
C
      IMPLICIT REAL*8 (A-H,O-Z)
      DIMENSION FCMX(*), FOMX(*) 
      integer, parameter :: N_SAVED = 5
      integer :: error, IFLAG, NBEST, N_USE
      integer, save :: ICALL
      logical, parameter :: LSUM = .FALSE., LSIGMAS = .FALSE.
      real(8), allocatable, save :: VALC(:,:), VALO(:,:), A(:), DA(:)

      if( .not. allocated(VALC) ) then
        allocate( VALC(N_SAVED,NMX), VALO(N_SAVED,NMX), 
     &            A(N_SAVED), DA(N_SAVED), stat = error )
        if( error > 0 ) stop ' XTRPF_AITKEN :: mem allocation error'
        VALC = 0.0d0;  VALO = 0.0d0
           A = 0.0d0;    DA = 0.d0;   ICALL = 0
      endif 
      
      ICALL = ICALL + 1
c      write(*,'(/a,2i4)') 'XTRPF_HURRY :: ICALL, IFLAG = ',ICALL,IFLAG
      
      if( ICALL > N_SAVED ) then
         do i = 1, N_SAVED-1
           VALC(i,1:NMX) = VALC(i+1,1:NMX)
           VALO(i,1:NMX) = VALO(i+1,1:NMX)
         enddo
         N_USE = N_SAVED
      else
         N_USE = ICALL
      endif
      
      ! fill in arrays VALC and VALO
      VALC(N_USE,1:NMX) = FCMX(1:NMX)
      VALO(N_USE,1:NMX) = FOMX(1:NMX)
      
      if( IFLAG == 0 ) return
      
      DO j = 1, NMX
         
         A(1:N_USE) = VALC(1:N_USE,j)
         call HURRY( A(1:N_USE), 2, N_USE, LSUM, F, NBEST, SERR, 
     &               LSIGMAS, DA )
         write(*,24) j, FCMX(j), F, A(1:N_USE)
24       format('j = ',i8,' FCMX(I/O) = ',1p,2e14.6,',  ',
     &                    ' SEQ: ',100e13.5)
         ! use new value only if it is not too far from the original one
c         D =  FCMX(j) - F
c         if( abs(D) < 1.d-8*ABS(FCMX(j)) ) FCMX(j) = F
         FCMX(j) = F
         
         A(1:N_USE) = VALO(1:N_USE,j)
c         write(*,20)  j, ICALL, A(1:ICALL)
         call HURRY( A(1:N_USE), 2, N_USE, LSUM, F, NBEST, SERR, 
     &               LSIGMAS, DA )
         write(*,25) FOMX(j), F, A(1:N_USE)
25       format( 12x,' FOMX(I/O) = ',1p,2e14.6,',  ',
     &               ' SEQ: ',100e13.5)        
         ! use new value only if it is not too far from the original one
c         D =  FOMX(j) - F
c         if( abs(D) < 1.d-8*ABS(FOMX(j)) ) FOMX(j) = F       
         FOMX(j) = F 
         
      ENDDO
            
      END SUBROUTINE XTRPF_HURRY
      
      !***********************************************************************
      ! DAMP FOCK MATRICES BY HARTREE DAMPING
      
      SUBROUTINE XTRPF( FCMX,FCMXM1, FOMX,FOMXM1, DFCTR, NMX )
      IMPLICIT REAL*8 (A-H,O-Z)
      DIMENSION FCMX(*),FCMXM1(*),FOMX(*),FOMXM1(*)
      DO N = 1, NMX
        FCMX(N) = ( 1.0D0 - DFCTR ) * FCMX(N) + DFCTR * FCMXM1(N)
        FOMX(N) = ( 1.0D0 - DFCTR ) * FOMX(N) + DFCTR * FOMXM1(N)
      ENDDO
      END
      !***********************************************************************

      SUBROUTINE EIGEN(EV,VC,N3,  NSYM,NBS,NOS,OCUP,  SMX,
     1  FCMX,FOMX,  ITER,  G,E,V,W,WORK,IW,  LOCVC,LOCMX)
C
      IMPLICIT REAL*8 (A-H,O-Z)
      DIMENSION EV(N3,*),VC(*)
      DIMENSION NBS(*),NOS(3,*),OCUP(2,*)
      DIMENSION SMX(*),FCMX(*),FOMX(*)
      DIMENSION G(*),E(*),V(*),W(*),WORK(*),IW(*)
      DIMENSION LOCVC(*),LOCMX(*)
      DATA THDIAG/1.0D-14/
C
C.... LOOP OVER SYMMETRY SPECIES
      DO L=1,NSYM
        IF(NOS(3,L).EQ.0) GO TO 10
C
C....   SOLVE EIGEN PROBLEM
        NC=LOCVC(L)+1
        MX=LOCMX(L)+1
        EPS=THDIAG
        CALL EIGEN1(EV(1,L),VC(NC),NBS(L)*2,NBS(L),NOS(1,L),NOS(2,L),
     1    OCUP(1,L),OCUP(2,L),  FCMX(MX),FOMX(MX),  ITER,
     2    SMX(MX),  G,E,V,W,WORK,IW,EPS)
   10   CONTINUE
      ENDDO
C
      END

      !***********************************************************************

      SUBROUTINE EIGEN1(EV,VC,NBST,NBS,NCSH,NOSH,OCUPC,OCUPO,
     1  FCMX,FOMX,  ITER,  SMX,  G,E,V,W,WORK,IW,EPS)
C
      IMPLICIT REAL*8 (A-H,O-Z)
      INTEGER*4 P,Q,PQ
      DIMENSION EV(*),VC(NBST,*)
      DIMENSION FCMX(*),FOMX(*),SMX(*)
      DIMENSION G(NBST,*),E(*),V(NBST,*),W(NBST,*),WORK(NBST,6),IW(*)
C
C.... THE FIRST SCF ITERATION, CLOSED-SHELL ONLY, OR OPEN-SHELL ONLY
      IF((ITER.GE.2).AND.(NCSH*NOSH.NE.0)) GO TO 16
      PQ=0
      DO P=1,NBST
       DO Q=1,P
        PQ=PQ+1
        IF(NCSH.NE.0) TERM=FCMX(PQ)
        IF(NOSH.NE.0) TERM=FOMX(PQ)
        G(P,Q)=TERM
        G(Q,P)=TERM
        V(P,Q)=SMX(PQ)
        V(Q,P)=SMX(PQ)
       ENDDO  
      ENDDO
      CALL GEIG(G,V,E,W,NBST,NBST,EPS,WORK,IW,NBST)
      GO TO 88
C
C.... GENERAL CASE (REF. O. MATSUOKA: J.PHYS. SOC. JPN 51 (1982) 2263)
C       IT IS ASSUMED THAT FOR EACH SYMMETRY SPECIES THERE IS ONLY
C       ONE OPEN SHELL, WHICH IS THE HIGHEST OCCUPIED SPINOR
C     FORM G-MATRIX
   16 CONTINUE
C          CLOSED - CLOSED, OPEN AND VIRTUAL
      CALL FORMG(VC,FCMX,G,W,  NBST,1,NBST,1,NCSH,2)
C          CLOSED - OPEN
      I=NCSH+1
      FCTR=OCUPC/(OCUPC-OCUPO)
      DO J=1,NCSH
        G(I,J)=G(I,J)*FCTR
      ENDDO
      FCTR=OCUPO/(OCUPC-OCUPO)
      PQ=0
      DO P=1,NBST
       DO Q=1,P
        PQ=PQ+1
        DO J=1,NCSH
          TERM=FOMX(PQ)*(VC(P,I)*VC(Q,J)+VC(Q,I)*VC(P,J))*FCTR
          IF(P.EQ.Q) TERM=TERM*0.5D0
          G(I,J)=G(I,J)-TERM
        ENDDO
       ENDDO  
      ENDDO  
C          OPEN - OPEN AND VIRTUAL
      CALL FORMG(VC,FOMX,G,W,  NBST,I,NBST,I,NBST,1)
C
C     SOLVE EIGEN PROBLEM
      CALL REIG(G,E,V,NBST,NBST,EPS,WORK,IW,NBST)
C
C     TRANSFORM VECTORS IN ORIGINAL BASIS SPINORS
      DO I=1,NBST
       DO P=1,NBST
        SUM=0.0D0
        DO J=1,NBST
          SUM=SUM+VC(P,J)*V(J,I)
        ENDDO 
        W(P,I)=SUM
       ENDDO
      ENDDO
C
C.... REARRANGE EIGEN VALUES AND VECTORS
   88 J=NBS
      DO I=1,NBS
        J=J+1
        EV(I)=E(J)
        DO P=1,NBST
          VC(P,I)=W(P,J)
        ENDDO
      ENDDO
      J=0
      DO I=NBS+1,NBST
        J=J+1
        EV(I)=E(J)
        DO P=1,NBST
          VC(P,I)=W(P,J)
        ENDDO
      ENDDO
C
      END

      !***********************************************************************

      SUBROUTINE FORMG(VC,FMX,G,W,  N,IMIN,IMAX,JMIN,JMAX,IND)
C
      IMPLICIT REAL*8 (A-H,O-Z)
      INTEGER*4 P,Q,PQ
      DIMENSION VC(N,*),FMX(*),G(N,*),W(N,*)
C
C.... FORM G-MATRIX
      DO Q=1,N
        DO I=IMIN,IMAX
          W(I,Q)=0.0D0
        ENDDO
      ENDDO
C
      PQ=0
      DO P=1,N
       DO Q=1,P
        PQ=PQ+1
        FPQ=FMX(PQ)
        DO I=IMIN,IMAX
          W(I,P)=W(I,P)+VC(Q,I)*FPQ
          IF(P.NE.Q) W(I,Q)=W(I,Q)+VC(P,I)*FPQ
        ENDDO
       ENDDO  
      ENDDO  
C
      DO I=IMIN,IMAX
        MAXJ=JMAX
        IF(IND.EQ.1) MAXJ=I
        DO J=JMIN,MAXJ
          SUM=0.0D0
          DO Q=1,N
            SUM=SUM+W(I,Q)*VC(Q,J)
          ENDDO
          G(I,J)=SUM
        ENDDO 
      ENDDO
C
      END

      !***********************************************************************

      SUBROUTINE NORMC(VC,SMX,  NSYM,NBS,NOS,LOCVC,LOCMX,
     1  W,IND,  *)
C
      IMPLICIT REAL*8 (A-H,O-Z)
      DIMENSION VC(*),SMX(*),NBS(*),NOS(3,*),LOCVC(*),LOCMX(*)
      DIMENSION W(*)
C
C.... LOOP OVER SYMMETRY SPECIES AND ORTHONORMALIZE SPINORS
      DO 10 L=1,NSYM
      IF(NOS(3,L).EQ.0) GO TO 10
      IF(IND.EQ.1) NNORM=NOS(3,L)
      IF(IND.EQ.2) NNORM=NBS(L)*2
C        IND = 1  ORTHONORMALIZE OCCUPIED SPINORS
C            = 2                 ALL SPINORS
      IF(NNORM.EQ.0) GO TO 10
      NC=LOCVC(L)+1
      NS=LOCMX(L)+1
      CALL NORMC1(VC(NC),SMX(NS),NBS(L)*2,NNORM,W,*20)
   10 CONTINUE
C
      RETURN
C
C.... PROCESS NORMALIZATION FAILURE
   20 WRITE(6,22)
   22 FORMAT(/'ERROR - NORMALIZATION OF SPINOR ORBITALS FAILED  ',
     1  '(SUBR NORMC)')
      RETURN 1
C
      END

      !***********************************************************************

      SUBROUTINE NORMC1(VC,SMX,NBST,NNORM,W,*)
C
      IMPLICIT REAL*8 (A-H,O-Z)
      INTEGER*4 P,Q,R,QR
      DIMENSION VC(NBST,*),SMX(*),W(*)
      DATA THNORM/1.0D-5/
C
C.... LOOP OVER SPINORS TO BE ORTHONORMALIZED
      DO 100 I=1,NNORM
C
C.... ORTHOGONALIZE SPINORS
      IF(I.EQ.1) GO TO 26
      DO Q=1,NBST
        W(Q)=0.0D0
      ENDDO
      QR=0
      DO Q=1,NBST
       DO R=1,Q
         QR=QR+1
         W(Q)=W(Q)+VC(R,I)*SMX(QR)
         W(R)=W(R)+VC(Q,I)*SMX(QR)
       ENDDO
       W(Q)=W(Q)-VC(Q,I)*SMX(QR)
      ENDDO
C
      JMAX=I-1
      DO J=1,JMAX
        B=0.0D0
        DO Q=1,NBST
          B=B+W(Q)*VC(Q,J)
        ENDDO
        DO P=1,NBST
          VC(P,I)=VC(P,I)-B*VC(P,J)
        ENDDO
      ENDDO
   26 CONTINUE
C
C.... NORMALIZE SPINORS
      QR=0
      T=0.0D0
      DO Q=1,NBST
        DO R=1,Q
         QR=QR+1
         T=T+VC(Q,I)*VC(R,I)*SMX(QR)*2.0D0
        ENDDO
        T=T-VC(Q,I)*VC(Q,I)*SMX(QR)
      ENDDO
C
C     TEST IF NORMALIZATION FAILED
      IF(T.LE.THNORM**2) RETURN 1
C
      T=1.0D0/DSQRT(T)
      DO P=1,NBST
        VC(P,I)=T*VC(P,I)
      ENDDO
C
  100 CONTINUE
C
      END

      !***********************************************************************

      SUBROUTINE PHASE(VC,SMX,VCM1,  NSYM,NBS,NOS,  LOCMX,LOCVC)
C
      IMPLICIT REAL*8 (A-H,O-Z)
      DIMENSION VC(*),SMX(*),VCM1(*)
      DIMENSION NBS(*),NOS(3,*),LOCMX(*),LOCVC(*)
C
C.... INQUIRE PHASES OF THE PRESENT AND THE PREVIOUS C-VECTORS
      DO 10 L=1,NSYM
        NV=LOCVC(L)+1
        NS=LOCMX(L)+1
        CALL PHASE1(VC(NV),SMX(NS),VCM1(NV),  NBS(L)*2,NOS(3,L))
   10 CONTINUE
C
      END

      !***********************************************************************

      SUBROUTINE PHASE1(VC,SMX,VCM1,  NBST,NOS)
C
      IMPLICIT REAL*8 (A-H,O-Z)
      INTEGER*4 P,Q,PQ
      DIMENSION VC(NBST,*),VCM1(NBST,*),SMX(*)
C
C.... LOOP OVER OCCUPIED SPINORS
      DO 100 I=1,NOS
C
C....   COMPUTE SCALAR PRODUCT OF THE PRESENT AND THE PREVIOUS VECTORS
        T=0.0D0
        PQ=0
        DO P=1,NBST
         DO Q=1,P
          PQ=PQ+1
          TERM=VC(P,I)*SMX(PQ)*VCM1(Q,I)
          T=T+TERM+TERM
         ENDDO
         T=T-TERM
        ENDDO 
C
C....   CHANGE THE PHASE OF THE PRESENT VECTOR, IF THE SCALAR IS NEGATIVE
        IF(T.LT.0.0D0) THEN
          DO P=1,NBST
           VC(P,I)=-VC(P,I)
          ENDDO
         END IF
  100 CONTINUE
C
      END

      !***********************************************************************

      SUBROUTINE CONVC(VC,VCM1,THCVL,THCVS,DIFVCL,DIFVCS,
     1  ETOT,ETOTM1,THCVEN,DIFFEN,
     2  NSYM,NBS,NOS,LOCVC,  ICONV,*)
C
      IMPLICIT REAL*8 (A-H,O-Z)
      INTEGER*4 P
      DIMENSION VC(*),VCM1(*),NBS(*),NOS(3,*),LOCVC(*)
C
C.... FIND THE MAX DIFF OF C-VECTORS
      DIFVCL=0.0D0
      DIFVCS=0.0D0
      DO 10 L=1,NSYM
        IF(NOS(3,L).EQ.0) GO TO 10
        IMAX=NOS(3,L)
        NBSL=NBS(L)
        N=LOCVC(L)
        DO I=1,IMAX
         DO P=1,NBSL*2
           N=N+1
           DIFF=DABS(VC(N)-VCM1(N))
           IF((P.LE.NBSL).AND.(DIFVCL.LT.DIFF)) DIFVCL=DIFF
           IF((P.GT.NBSL).AND.(DIFVCS.LT.DIFF)) DIFVCS=DIFF
         ENDDO    
        ENDDO
   10 CONTINUE
C
C.... QUERY CONVERGENCE ON C-VECTORS AND ENERGY
      DIFFEN=DABS(ETOT-ETOTM1)
      IF(DIFFEN.LE.THCVEN) ICONV=3
      IF((DIFVCL.LE.THCVL).AND.(DIFVCS.LE.THCVS)) ICONV=2
      IF((ICONV.EQ.2).OR.(ICONV.EQ.3)) RETURN 1
C
C.... NOT CONVERGED
      RETURN
C
      END

      !***********************************************************************

      SUBROUTINE XTRPC(VC,VCM1,DFCTR,  NSYM,NBS,NOS,LOCVC)
C
      IMPLICIT REAL*8 (A-H,O-Z)
      DIMENSION VC(*),VCM1(*)
      DIMENSION NBS(*),NOS(3,*),LOCVC(*)
C
C.... LOOP OVER SYMMETRY SPECIES
      DO 10 L=1,NSYM
        IF(NOS(3,L).EQ.0) GO TO 10
C
C....   DAMP OCCUPIED C-VECTORS BY HARTREE DAMPING
        NMIN = LOCVC(L) + 1
        NMAX = LOCVC(L) + NOS(3,L)*NBS(L)*2
        DO N = NMIN, NMAX
          VC(N) = ( 1.0D0 - DFCTR )* VC(N) + DFCTR * VCM1(N)
        ENDDO
   10 CONTINUE
C
      END

      !***********************************************************************

      SUBROUTINE CONVD(DMX,DMXM1,  NSYM,NBS,NOS,  LOCMX,
     1  THDLL,THDSL,THDSS,  DIFDLL,DIFDSL,DIFDSS,  ICONV,  *)
C
      IMPLICIT REAL*8 (A-H,O-Z)
      INTEGER*4 P,Q
      DIMENSION DMX(*),DMXM1(*)
      DIMENSION NBS(*),NOS(3,*),LOCMX(*)
C
C.... FIND MAX DIFF OF D-MATRICES
      DIFDLL=0.0D0
      DIFDSL=0.0D0
      DIFDSS=0.0D0
      DO 10 L=1,NSYM
       IF(NOS(3,L).EQ.0) GO TO 10
       NBSL=NBS(L)
       N=LOCMX(L)
       DO P=1,NBSL*2
        DO Q=1,P
         N=N+1
         DIFF=DABS(DMX(N)-DMXM1(N))
         IF((P.LE.NBSL).AND.(DIFDLL.LT.DIFF)) DIFDLL=DIFF
         IF((P>NBSL).AND.(Q.LE.NBSL).AND.(DIFDSL<DIFF)) DIFDSL=DIFF
         IF((Q.GT.NBSL).AND.(DIFDSS.LT.DIFF)) DIFDSS=DIFF
        ENDDO  
       ENDDO
   10 CONTINUE
C
C.... QUERY CONVERGENCE ON DENSITY MATRICES
      IF((DIFDLL.LE.THDLL).AND.(DIFDSL.LE.THDSL).AND.(DIFDSS.LE.THDSS))
     1  ICONV=1
      IF(ICONV.EQ.1) RETURN 1
C
C.... NOT CONVERGED
      RETURN
C
      END

      !***********************************************************************

      SUBROUTINE SCFOUT(EV,VC,N3,  ETOT,EMASS,EKIN,EPOT,  C,
     1  TITLE, ZNUC,RNUC,ALPHA,NUCMDL,
     2  NSYM,NBS,NOS, NCONF,WAV,OCUPAV,
     3  DIFVCL,DIFVCS,DIFFEN,DIFDLL,DIFDSL,DIFDSS,ICONV,ITER,
     4  LOCVC,  IPRVC,IPRMX,SMX,HMX,DTMX,DOMX,FCMX,FOMX,  LOCMX,LOCTR,
     5  REXP,N6,  NPBS,ZETA,CBS,N1,N5,  NTERM,NQNTM,CNORM,
     6  NFT,  IERR)
C
      IMPLICIT REAL*8 (A-H,O-Z)
      DIMENSION EV(N3,*),VC(*)
      CHARACTER*8 TITLE(20)
      DIMENSION NBS(*),NOS(3,*),LOCVC(*)
      DIMENSION WAV(*),OCUPAV(7,*)
      DIMENSION SMX(*),HMX(*),DTMX(*),DOMX(*),FCMX(*),FOMX(*)
      DIMENSION LOCMX(*),LOCTR(*)
      DIMENSION REXP(-2:4,N6,*)
      DIMENSION NPBS(N1,*),ZETA(N5,N1,*),CBS(N5,N1,2,*)
      DIMENSION NTERM(2,*),NQNTM(2,2,*),CNORM(2,N5,N1,2,*)
      CHARACTER*4 LABEL(7)
      DATA LABEL/'S+  ','P-  ','P+  ','D-  ','D+  ','F-  ','F+  '/
C
C.... PRINT TITLE
      WRITE(6,10) TITLE
   10 FORMAT(//'COMPUTED RESULTS ',62('-')//'PROPHET-DFR-ATOM (CGTF)'/
     1  'ATOMIC DIRAC-FOCK-ROOTHAAN SCF PROGRAM'//10A8/10A8)
      IF(IERR.NE.0) WRITE(6,12)
   12 FORMAT(/30X,72('*')//
     1  39X,'COMPUTATION INTERRUPTED DUE TO ERROR DETECTION'/
     3  /30X,72('*'))
C
      WRITE(6,14) ZNUC
   14 FORMAT(/'NUCLEAR CHARGE',F12.5)
      IF(NUCMDL.EQ.1) WRITE(6,151)
      IF(NUCMDL.EQ.2) WRITE(6,152)
      IF(NUCMDL.EQ.3) WRITE(6,153)
  151 FORMAT(/'NUCLEAR MODEL: POINT NUCLEUS')
  152 FORMAT(/'NUCLEAR MODEL: FINITE SPHERE NUCLEUS')
  153 FORMAT(/'NUCLEAR MODEL: GAUSSIAN NUCLEUS')
C
      WRITE(6,17) C
   17 FORMAT(/'SPEED OF LIGHT',D22.12)
C
      WRITE(6,16) (LABEL(L),L=1,NSYM)
   16 FORMAT(/23X,7(3X,A2))
      WRITE(6,18) (NOS(1,L),L=1,NSYM)
   18 FORMAT('NUMBER OF CLOSED SHELLS',7I5)
      WRITE(6,20) (NOS(2,L),L=1,NSYM)
   20 FORMAT(10X,'OPEN SHELLS  ',7I5)
C
C.... PRINT ENERGIES AND COMPUTE VIRIAL FOR POINT CHARGE
C     OR FINITE-SPHERE NUCLEUS ATOM
      WRITE(6,24) ETOT,EMASS,EKIN,EPOT
   24 FORMAT(/'ENERGIES',/,1p,
     1  ' TOTAL      ',D26.14/' REST MASS  ',D26.14/
     2  ' KINETIC <T>',D26.14/' POTENTIAL <V>',D24.14)
C
      IF(NUCMDL.EQ.1) THEN
      VIR=EPOT/EKIN
      WRITE(6,22) VIR
   22 FORMAT(/'VIRIAL <V>/<T>',D24.14)
      ELSE IF(NUCMDL.EQ.2) THEN
      CALL VIRIAL(VIR,  EKIN,EPOT,  ZNUC,RNUC,
     1  NSYM,NBS,NPBS,ZETA,CBS,N1,N5,
     2  NTERM,NQNTM,CNORM,  DTMX,LOCMX,LOCTR)
      WRITE(6,23) VIR
   23 FORMAT(/'VIRIAL (<V>+CORRECTION)/<T>',D24.14)
C       FOR THE ABOVE CORRECTION TERM, SEE MATSUOKA AND KOGA:
C       PHYS. REV. A. XX (2000) XXX.
      END IF
C
C.... PRINT CONVERGENCE DATA
      WRITE(6,28) ITER
   28 FORMAT(/'SCF ITERATION NO',I6)
      IF(ICONV.EQ.  1) WRITE(6,281)
      IF(ICONV.EQ.  2) WRITE(6,282)
      IF(ICONV.EQ.  3) WRITE(6,283)
      IF(ICONV.EQ.100) WRITE(6,284)
      IF(ICONV.EQ.  0) WRITE(6,285)
  281 FORMAT(/'CONVERGED ON TOTAL D-MATRIX')
  282 FORMAT(/'CONVERGED ON C-VECTORS')
  283 FORMAT(/'CONVERGED ON TOTAL ENERGY')
  284 FORMAT(/'NO CONVERGENCE WITHIN THE MAX ITERATIONS')
  285 FORMAT(/'SCF ITER INTERRUPTED (PROPABLY NOT YET CONVERGED)')
      WRITE(6,26) DIFVCL,DIFVCS,DIFFEN,DIFDLL,DIFDSL,DIFDSS
   26 FORMAT(/'CONVERGENCE THRESHOLDS',/,1p,
     1  ' COEF(LARG)',D11.3/' COEF(SMAL)',D11.3/
     2  ' TOTL ENRGY',D11.3/' D(LL)-MTRX',D11.3/
     3  ' D(SL)-MTRX',D11.3/' D(SS)-MTRX',D11.3)
C
C.... PRINT SPINOR ENERGIES, C-VECTORS, AND MATRICES
      CALL PRTVC(EV,VC,N3, NSYM,NBS,NOS,LOCVC,IPRVC)
C
      IF(IPRMX.NE.0) THEN
      CALL PRTMX(SMX,NSYM,NBS,'S       ')
      CALL PRTMX(HMX,NSYM,NBS,'H       ')
      CALL PRTMX(DTMX,NSYM,NBS,'D(TOTAL)')
      CALL PRTMX(DOMX,NSYM,NBS,'D(OPEN) ')
      CALL PRTMX(FCMX,NSYM,NBS,'F(CLOS) ')
      CALL PRTMX(FOMX,NSYM,NBS,'F(OPEN) ')
C     IF OPEN SHELL IS NOT OCCUPIED, FOMX=FCMX (SEE SUBR FORMF)
      END IF
C
C.... COMPUTE RADIAL EXPECTATION VALUES
      CALL RADEX(REXP,N6,  NSYM,NBS,NPBS,ZETA,CBS,N1,N5,
     1  NTERM,NQNTM,CNORM,  NOS,VC,LOCVC)
C
      WRITE(6,40) (N,N=-2,-1),(N,N=1,4)
   40 FORMAT(/'RADIAL EXPECTATION VALUES'//
     1  'SYM  ORB',3(7X,'<R**',I2,'>',6X)/9X,3(7X,'<R**',I2,'>',6X))
      DO L=1,NSYM
       DO I=1,NOS(3,L)
        IF(I.EQ.1) WRITE(6,44) LABEL(L),I,(REXP(N,I,L),N=-2,-1),
     1                                    (REXP(N,I,L),N= 1, 4)
        IF(I.NE.1) WRITE(6,46) I,(REXP(N,I,L),N=-2,-1),
     1                           (REXP(N,I,L),N= 1, 4)
   44   FORMAT(A2,I5,3D20.12/8X,3D20.12)
   46   FORMAT(2X,I5,3D20.12/8X,3D20.12)
       ENDDO
      ENDDO
C
C.... SAVE SCF RESULTS
      IF(NFT.NE.0) CALL SAVE(TITLE,  ZNUC,RNUC,ALPHA,NUCMDL,
     1  NSYM,NBS,NOS,  NCONF,WAV,OCUPAV,  NPBS,ZETA,CBS,N1,N5,
     2  ETOT,EMASS,EKIN,EPOT,VIR,
     3  ICONV,DIFVCL,DIFVCS,DIFFEN,DIFDLL,DIFDSL,DIFDSS,
     4  EV,VC,N3,  LOCVC,  REXP,N6,  NFT)
C
      END

      !***********************************************************************

      SUBROUTINE PRTVC(EV,VC,N3,  NSYM,NBS,NOS,LOCVC,IND)
C
      IMPLICIT REAL*8 (A-H,O-Z)
      CHARACTER*4 LABEL
      DIMENSION EV(N3,*),VC(*),NBS(*),NOS(3,*),LOCVC(*)
      DIMENSION LABEL(7)
      DATA LABEL/'S+  ','P-  ','P+  ','D-  ','D+  ','F-  ','F+  '/
C
C.... PRINT HEADING
      IF(IND.LT.4) WRITE(6,10)
   10 FORMAT(/'SPINOR ENERGIES AND C-VECTORS')
C
C.... PRINT SPINOR ENERGIES AND VECTORS
      DO 20 L=1,NSYM
        IF(NOS(3,L).EQ.0) GO TO 20
        NC=LOCVC(L)+1
        NUP=L/2
        CALL PRVC1(LABEL(L),NUP,EV(1,L),VC(NC),NOS(3,L),
     1    NBS(L),NBS(L)*2,IND)
   20 CONTINUE
C
      END

      !***********************************************************************

      SUBROUTINE PRVC1(LABEL,NUP,EV,VC,NOS,NBS,NBST,IND)
C
      IMPLICIT REAL*8 (A-H,O-Z)
      CHARACTER*4 BLANK,STAR,LINE,VIRTL,LABEL
      INTEGER*4 P
      DIMENSION EV(*),VC(NBST,*),VIRTL(8)
      DATA BLANK/'    '/,STAR/'*   '/,LINE/'----'/
C
C.... PRINT SPINOR ENERGIES AND VECTORS
      IF(IND.EQ.1) IMAX=NOS
      IF(IND.EQ.2) IMAX=NBS
      IF(IND.EQ.3) IMAX=NBST
      IF(IND.EQ.4) IMAX=NBST
      IF(IND.EQ.5) IMAX=NOS
C        IND = 1  PRINT OCCUPIED SPINORS
C            = 2        ALL SPINORS OF ELECTRONS
C            = 3        ALL SPINORS OF ELECTRONS AND POSITRONS
C            = 4        ALL SPINORS BUT NOT ENERGIES
C            = 5        OCCUPIED SPINORS BUT NOT ENERGIES
      DO 10 I=1,IMAX,8
      JMAX=MIN0(IMAX,I+7)
      K=0
      DO 14 J=I,JMAX
      K=K+1
C     MARK VIRTUAL SPINORS BY SYMBOLS *
      IF(J.LE.NOS) VIRTL(K)=BLANK
      IF(J.GT.NOS) VIRTL(K)=STAR
   14 CONTINUE
      WRITE(6,16) (J+NUP,LABEL,VIRTL(J-I+1),J=I,JMAX)
   16 FORMAT(/3X,8(5x,I7,A2,A4,2X,4x))
C
      IF(IND.GE.4) GO TO 28
      WRITE(6,20) (EV(J),J=I,JMAX)
   20 FORMAT(3X,8(1P,D24.14))
      WRITE(6,22) (LINE,LINE,LINE,LINE,LINE,J=I,JMAX)
   22 FORMAT(3X,8(4X,5A4))
   28 CONTINUE
C
      WRITE(6,27)
      DO 24 P=1,NBST
      WRITE(6,26) P,(VC(P,J),J=I,JMAX)
   26 FORMAT(I3,8(1P,D24.14))
      IF(P.EQ.NBS) WRITE(6,27)
   27 FORMAT()
   24 CONTINUE
   10 CONTINUE
C
      END

      !***********************************************************************

      SUBROUTINE PRTMX(A,NSYM,NBS,ANAME)
C
      IMPLICIT REAL*8 (A-H,O-Z)
      CHARACTER*8 ANAME
      CHARACTER*4 LABEL
      INTEGER*4 P
      DIMENSION A(*),NBS(*),LABEL(7)
      DATA LABEL/'S+  ','P-  ','P+  ','D-  ','D+  ','F-  ','F+  '/
C
C.... PRINT MATRIX NAME
      WRITE(6,10) ANAME
   10 FORMAT(/'MATRIX - ',A8)
C
C.... LOOP OVER SYMMETRY SPECIES
      NMAX=0
      DO 100 L=1,NSYM
C
C.... PRINT MATRIX
      WRITE(6,30) LABEL(L)
   30 FORMAT(/'SYMMETRY SPECIES',2X,A4/)
      DO 32 P=1,NBS(L)*2
      NMIN=NMAX+1
      NMAX=NMAX+P
      WRITE(6,34) P,(A(N),N=NMIN,NMAX)
   34 FORMAT(I3,8(1PD15.7),:/(3X,8(1PD15.7)))
   32 CONTINUE
C
  100 CONTINUE
C
      END

      !***********************************************************************

      SUBROUTINE VIRIAL(VIR,  EKIN,EPOT,  ZNUC,RNUC,
     1  NSYM,NBS,NPBS,ZETA,CBS,N1,N5,
     2  NTERM,NQNTM,CNORM,  DTMX,LOCMX,LOCTR)
C
      IMPLICIT REAL*8 (A-H,O-Z)
      INTEGER*4 P,Q
      DIMENSION NBS(*),NPBS(N1,*),ZETA(N5,N1,*),CBS(N5,N1,2,*)
      DIMENSION NTERM(2,7),NQNTM(2,2,7),CNORM(2,N5,N1,2,*)
      DIMENSION DTMX(*),LOCMX(*),LOCTR(*)
      DIMENSION XW(2)
      COMMON /FNUT/F(0:8),TABF(0:14,0:120)
      DATA UFCTR/0.79788 45608 02865D0/
C                =SQRT(2/PI)
C
C.... LOOP OVER SYMMETRY SPECIES AND BASIS SPINORS
      ECORR=0.0D0
      RNUC2=RNUC**2
      RNUC4=RNUC2*4.0D0
C
      DO 100 L=1,NSYM
C
        NBSL=NBS(L)
        DO LP=1,NBSL
         DO LQ=1,LP
C
C....     COMPUTE CORRECTION INTEGRALS
          XW(1)=0.0D0
          XW(2)=0.0D0
C
          NUMAX=NQNTM(NTERM(2,L),2,L)+1
          DO P=1,NPBS(LP,L)
           DO Q=1,NPBS(LQ,L)
C
            XI=(ZETA(P,LP,L)+ZETA(Q,LQ,L))*RNUC2
            CALL AUXF(NUMAX,XI)
C
            DO LS=1,2
             SUM=0.0D0
             DO IP=1,NTERM(LS,L)
              DO IQ=1,NTERM(LS,L)
               NU=(NQNTM(IP,LS,L)+NQNTM(IQ,LS,L))/2
               SUM=SUM+(F(NU)-F(NU+1))*(RNUC4**NU)
     1                 *CNORM(IP,P,LP,LS,L)*CNORM(IQ,Q,LQ,LS,L)
              ENDDO     
             ENDDO
           
             XW(LS)=XW(LS)+SUM*CBS(P,LP,LS,L)*CBS(Q,LQ,LS,L)
            ENDDO
           ENDDO
          ENDDO
C
          XW(1)=XW(1)*ZNUC*UFCTR*3.0D0
          XW(2)=XW(2)*ZNUC*UFCTR*3.0D0
C
C....     COMPUTE CORRECTION TO THE ONE-ELECTRON POTENTIAL
          LPQLL=LOCMX(L)+LOCTR(LP)+LQ
          LPQSS=LOCMX(L)+LOCTR(LP+NBSL)+(LQ+NBSL)
          TERM=XW(1)*DTMX(LPQLL)+XW(2)*DTMX(LPQSS)
          IF(LP.NE.LQ) TERM=TERM+TERM
C
          ECORR=ECORR+TERM
         ENDDO  
        ENDDO    
  100 CONTINUE
C
C.... COMPUTE VIRIAL RATIO FOR FINITE-SPHERE NUCLEUS ATOM
      VIR = (EPOT+ECORR)/EKIN
C
      END

      !***********************************************************************

      SUBROUTINE RADEX(REXP,N6,  NSYM,NBS,NPBS,ZETA,CBS,N1,N5,
     1  NTERM,NQNTM,CNORM,  NOS,VC,LOCVC)
C
      IMPLICIT REAL*8 (A-H,O-Z)
      INTEGER*4 P,Q
      DIMENSION REXP(-2:4,N6,*)
      DIMENSION NBS(*),NPBS(N1,*),ZETA(N5,N1,*),CBS(N5,N1,2,*)
      DIMENSION NTERM(2,7),NQNTM(2,2,7),CNORM(2,N5,N1,2,*)
      DIMENSION NOS(3,*),VC(*),LOCVC(*)
      DIMENSION V(0:20),XV(-2:4,2),XVPQ(-2:4)
C
C.... INITIALIZE STORAGES TO ZERO
      DO L=1,NSYM
       DO I=1,NOS(3,L)
        DO N=-2,4
         REXP(N,I,L)=0.0D0
        ENDDO 
       ENDDO  
      ENDDO  
C
C.... LOOP OVER SYMMETRY SPECIES AND BASIS SPINORS
      DO 100 L=1,NSYM
        IF(NOS(3,L).EQ.0) GO TO 100
        NPQNMX=NQNTM(NTERM(2,L),2,L)*2+4
        DO LP=1,NBS(L)
         DO LQ=1,LP
C
C....     COMPUTE V-INTEGRALS OVER BASIS SPINORS
          DO N=-2,4
           XV(N,1)=0.0D0
           XV(N,2)=0.0D0
          ENDDO
C
          DO P=1,NPBS(LP,L)
           DO Q=1,NPBS(LQ,L)
C
            ZPQ=(ZETA(P,LP,L)+ZETA(Q,LQ,L))*0.5D0
            CALL AUXV1(NPQNMX,ZPQ,V)
            DO 20 LS=1,2
             CALL XINTV(XVPQ(-2),  V,NTERM(LS,L),NQNTM(1,LS,L),
     1         CNORM(1,P,LP,LS,L),CNORM(1,Q,LQ,LS,L))
             CC=CBS(P,LP,LS,L)*CBS(Q,LQ,LS,L)
             DO N=-2,4
               XV(N,LS)=XV(N,LS)+XVPQ(N)*CC
             ENDDO 
   20       CONTINUE   
           ENDDO
          ENDDO
C
C....     COMPUTE RADIAL EXPECTATION VALUES
          NC=LOCVC(L)
          NBSL=NBS(L)
          DO 30 I=1,NOS(3,L)
           VCL=VC(LP+NC)*VC(LQ+NC)
           VCS=VC(LP+NBSL+NC)*VC(LQ+NBSL+NC)
           IF(LP.NE.LQ) VCL=VCL+VCL
           IF(LP.NE.LQ) VCS=VCS+VCS
           DO N=-2,4
            REXP(N,I,L)=REXP(N,I,L)+XV(N,1)*VCL+XV(N,2)*VCS
           ENDDO
           NC=NC+NBSL*2
   30     CONTINUE
C        
         ENDDO
        ENDDO
  100 CONTINUE
C
      END

      !***********************************************************************

      SUBROUTINE XINTV(XV,  V,NTERM,NQNTM,CP,CQ)
C
      IMPLICIT REAL*8 (A-H,O-Z)
      INTEGER*4 P,Q
      DIMENSION XV(-2:*),V(0:*),NQNTM(*),CP(*),CQ(*)
      DATA UFCTR/0.79788 45608 02865D0/
C               = SQRT(2/PI)
C
C.... COMPUTE V-INTEGRAL OVER PRIMITIVE BASIS SPINORS
      DO N=-2,4
        XV(N)=0.0D0
      ENDDO
C
      DO P=1,NTERM
       DO Q=1,NTERM
         DO N=-2,4
           NPQN=NQNTM(P)+NQNTM(Q)+N
           XV(N)=XV(N)+V(NPQN)*CP(P)*CQ(Q)
         ENDDO
       ENDDO   
      ENDDO   
C
      DO N=-2,4
        XV(N)=XV(N)/(2.0D0**N)
      ENDDO
      
      DO N=-1,3,2
        XV(N)=XV(N)*UFCTR
      ENDDO
C
      END

      !***********************************************************************

      SUBROUTINE SAVE(TITLE,  ZNUC,RNUC,ALPHA,NUCMDL,
     1  NSYM,NBS,NOS,  NCONF,WAV,OCUPAV,  NPBS,ZETA,CBS,N1,N5,
     2  ETOT,EMASS,EKIN,EPOT,VIRIAL,
     3  ICONV,DIFVCL,DIFVCS,DIFFEN,DIFDLL,DIFDSL,DIFDSS,
     4  EV,VC,N3,  LOCVC,  REXP,N6,  NFT)
C
      IMPLICIT REAL*8 (A-H,O-Z)
      INTEGER*4 P
      CHARACTER*8 TITLE(20)
      DIMENSION NBS(*),NOS(3,*),WAV(*),OCUPAV(7,*)
      DIMENSION REXP(-2:4,N6,*)
      DIMENSION NPBS(N1,*),ZETA(N5,N1,*),CBS(N5,N1,2,*)
      DIMENSION EV(N3,*),VC(*),LOCVC(*)
      CHARACTER*4 LABEL(7)
      DATA LABEL/'S+  ','P-  ','P+  ','D-  ','D+  ','F-  ','F+  '/
C
C.... SAVE SCF RESULTS
C       E.G., "RECFM(F,B),LRECL(80),BLKSIZE(1600)" MAY BE ADOPTED.
      REWIND NFT
C
      WRITE(NFT,10) TITLE
   10 FORMAT(10A8)
      WRITE(NFT,20) ZNUC,RNUC,ALPHA,NUCMDL
cAV
   20 FORMAT( 3D24.15, I5 )
  201 FORMAT( 3D24.15, F14.8 ) 
cendAV

      WRITE(NFT,30) NSYM,(NBS(L),L=1,NSYM)
   30 FORMAT(20I4)
      WRITE(NFT,30) ((NOS(I,L),I=1,3),L=1,NSYM)
C
      WRITE(NFT,30) NCONF
      DO I=1,NCONF
cAV      write(*,*) 'i, wav(i) = ',I,WAV(I)
cAV      write(*,*) 'NSYM = ',NSYM
cAV      write(*,*) (OCUPAV(L,I),L=1,NSYM)
        WRITE(NFT,201) WAV(I),(OCUPAV(L,I),L=1,NSYM)
      ENDDO
C
      DO 32 L=1,NSYM
        WRITE(NFT,30) (NPBS(LP,L),LP=1,NBS(L))
        DO LP=1,NBS(L)
          WRITE(NFT,201) (ZETA(P,LP,L),P=1,NPBS(LP,L))
        ENDDO
        DO LS=1,2
         DO LP=1,NBS(L)
          WRITE(NFT,201) (CBS(P,LP,LS,L),P=1,NPBS(LP,L))
         ENDDO
        ENDDO 
   32 CONTINUE
C
      WRITE(NFT,201) ETOT,EMASS,EKIN,EPOT,VIRIAL
C
      WRITE(NFT,30) ICONV
      WRITE(NFT,201) DIFVCL,DIFVCS,DIFFEN,DIFDLL,DIFDSL,DIFDSS
C
      DO 50 L=1,NSYM
        IF(NOS(3,L).EQ.0) GO TO 50
        NC=LOCVC(L)+1
        WRITE(NFT,60) LABEL(L)
   60   FORMAT(A4)
c**      CALL SAVE1(EV(1,L),VC(NC),NBS(L),NBS(L)*2,NFT)
        CALL SAVE1(EV(1,L),VC(NC),NOS(3,L),NBS(L)*2,NFT)
   50 CONTINUE
C
      DO L=1,NSYM
       DO I=1,NOS(3,L)
         WRITE(NFT,201) (REXP(N,I,L),N=-2,4)
       ENDDO 
      ENDDO
C
      REWIND NFT
C
      END

      !***********************************************************************

      SUBROUTINE SAVE1(EV,VC,NOS,NBST,NFT)
C
      IMPLICIT REAL*8 (A-H,O-Z)
      DIMENSION EV(*),VC(NBST,*)
C
C.... SAVE EIGEN VALUES AND VECTORS
      WRITE(NFT,20) (EV(I),I=1,NOS)
   20 FORMAT(3D24.15)
      DO 10 I=1,NOS
      WRITE(NFT,30) (VC(LP,I),LP=1,NBST)
   30 FORMAT(3(D24.15,','))
   10 CONTINUE
C
      END

      !***********************************************************************
      ! prints out ( N x N ) matrix/vector with comment to unit nout
      
      subroutine matrix_out( xm, n, string, nout )
      implicit double precision (a-h,o-z)
      dimension xm(n,n)
      character*(*) string
c
      write( nout, '(/a)' ) string
      do i = 1, n
         write(nout,'(1p,100e16.6)')  xm(i,1:n)
      enddo
      return
      end

      !***********************************************************************

      SUBROUTINE GEIG(A,B, E,V, N,NE,EPS, W,NW, N1)
C
      IMPLICIT REAL*8 (A-H,O-Z)
      DIMENSION A(N1,N),B(N1,N),E(N),V(N1,N)
      DIMENSION W(N1,6),NW(N)
cAV
      integer, parameter :: ITYPE = 1 ! A*x = (lambda)*B*x
      character, save :: JOBZ = 'V',  ! Compute eigenvalues and eigenvectors.
     &                   UPLO = 'U',  ! Upper triangles of A and B are stored
     &                   RANG = 'A'   ! all eigenvalues will be found.
      integer :: error, INFO
      integer, save :: LWORK
      integer, allocatable, save :: IWORK2(:), IFAIL(:)
      real(8), allocatable, save :: W2(:), WORK2(:), Z(:,:)
cendAV
C.... SOLVE GENERALIZED EIGEN EQUATION AV = EBV
      IF((N.LE.0).OR.(NE.LE.0).OR.(N.GT.N1).OR.(NE.GT.N)) THEN
         WRITE(6,*) 'ERROR(GEIG) - INPUT: N,NE,N1',N,NE,N1
         RETURN
      ENDIF
cAV         
c      write(*,'(/a,3i8)') ' GEIG :: N, NE, N1 = ',N, NE, N1
c      write(*,*) '   dimensions of A and B = ',N1,N
c      write(*,*) '   matrix A : '
c      do i = 1, N1
c        write(*,'(1p,100e16.6)')  A(i,1:N)
c      enddo
c      write(*,*) '   matrix B : '
c      do i = 1, N1
c        write(*,'(1p,100e16.6)')  B(i,1:N)
c      enddo

      goto 100

      ! 1: DSYGV (QR iteration)
      ! 2: DSYGVD (divide and conquer)
      ! 3: DSYGVX (bisection) -> ISSUES W/EIGENVECTORS (PHASES)
      iSUB = 2

      LWORK = max( 3*N-1, 1+6*N+2*N**2, 8*N )
      LIWORK = 3 + 5*N

      if( .not. allocated(W2) ) then
        allocate( W2(N), WORK2(LWORK), IWORK2(LIWORK), Z(N,N), 
     &            IFAIL(N), stat = error )
        if( error > 0 ) stop 'GEIG :: memory allocation error'
      endif

      INFO = 0;  
      W2 = 0.d0;  WORK2 = 0.d0;  IWORK2 = 0;  Z = 0.d0; IFAIL = 0 
      
      select case( iSUB )
        case( 1 )       
          call DSYGV( ITYPE, JOBZ, UPLO, N, A, N1, B, N1, W2, WORK2,
     &                LWORK, INFO )
          E(1:N) = W2(1:N)          ! copy eigenvalues from W2 to E
          V(1:N1,1:N) = A(1:N1,1:N) ! copy eigenvectors from A to V
        case( 2 ) 
          call DSYGVD( ITYPE, JOBZ, UPLO, N, A, N1, B, N1, W2, WORK2,
     &                 LWORK, IWORK2, LIWORK, INFO  )
          E(1:N) = W2(1:N)          ! copy eigenvalues from W2 to E
          V(1:N1,1:N) = A(1:N1,1:N) ! copy eigenvectors from A to V
        case( 3 ) 
          ABSTOL = 2.0D0 * DLAMCH('S')
          call DSYGVX( ITYPE, JOBZ, RANG, UPLO, N, A, N1, B, N1, VL, VU, 
     &                 IL, IU, ABSTOL, M, W2, Z, N1, WORK2, LWORK, 
     &                 IWORK2(1:5*N), IFAIL, INFO  )
          E(1:N) = W2(1:N)          ! copy eigenvalues from W2 to E
          V(1:N1,1:N) = Z(1:N1,1:N) ! copy eigenvectors from Z to V
        case default
          stop ' GEIG :: iSUB < 1 or iSUB > 3 '    
      end select 

c      write(*,*) ' DSYGV returned INFO = ',INFO
      if( INFO / = 0 ) then
        write(*,*) ' DSYGV/DSYGVD/DSYGVX returned INFO = ',INFO
        stop
      endif
c      write(*,*) '   eigenvalues :'
c      write(*,'(1p,100e16.6)')  E(1:N)
c      write(*,*) '   eigenvectors :'
c      do i = 1, N1
c         write(*,'(1p,100e16.6)')  V(i,1:N)
c      enddo
c      stop
      return
c
100   continue
cendAV      
      IF(N.NE.1) THEN
C
C.... MODIFIED CHOLESKY FACTORIZATION
         DO K=N,1,-1
            S=0.0D0
            DO I=K+1,N
               S=S+W(I,1)*B(I,K)*B(I,K)
            END DO
            D=B(K,K)-S
            IF(D.LE.0.0D0) THEN
               WRITE(6,10)
               RETURN
            END IF
            W(K,1)=D
            D=1.0D0/D
            B(K,K)=DSQRT(D)
            DO J=1,K-1
               S=0.0D0
               DO I=K+1,N
                  S=S+W(I,1)*B(I,K)*B(I,J)
               END DO
               B(K,J)=(B(K,J)-S)*D
            END DO
         END DO
C
         DO K=N-1,1,-1
            DO J=1,K-1
               S=0.0D0
               DO I=K+1,N
                  S=S+B(I,K)*A(I,J)
               END DO
               A(K,J)=A(K,J)-S
            END DO
            S0=0.0D0
            DO J=K+1,N
               S=0.0D0
               DO I=K+1,J-1
                  S=S+B(I,K)*A(J,I)
               END DO
               DO I=J,N
                  S=S+B(I,K)*A(I,J)
               END DO
               T=A(J,K)-S
               S0=S0+B(J,K)*(A(J,K)+T)
               A(J,K)=T
            END DO
            A(K,K)=A(K,K)-S0
         END DO
C
         DO I=1,N
            A(I,I)=A(I,I)/W(I,1)
            T=B(I,I)
            DO J=1,I-1
               A(I,J)=A(I,J)*B(J,J)*T
            END DO
         END DO
C
C.... FIND EIGENVALUES AND EIGENVECTORS
         CALL REIG(A, E,V, N,NE,EPS, W,NW, N1)
c
c         write(*,*) '   eigenvalues :'
c         write(*,'(1p,100e16.6)')  E(1:N)
c         write(*,*) '   eigenvectors :'
c         do i = 1, N1
c           write(*,'(1p,100e16.6)')  V(i,1:N)
c         enddo
C
         DO J=1,NE
            DO I=1,N
               V(I,J)=V(I,J)*B(I,I)
            END DO
            DO K=1,N-1
               T=V(K,J)
               DO I=K+1,N
                  V(I,J)=V(I,J)-T*B(I,K)
               END DO
            END DO
         END DO
c         write(*,*) '   eigenvectors :'
c         do i = 1, N1
c           write(*,'(1p,100e16.6)')  V(i,1:N)
c         enddo
c         stop
         RETURN
      ELSE
         T=B(1,1)
         IF(T.LE.0) THEN
            WRITE(6,10)
 10         FORMAT('ERROR(GEIG) - MATRIX B IS NOT POSITIVE DEFINITE.')
            RETURN
         END IF
         E(1)=A(1,1)/T
         V(1,1)=1.0D0
         RETURN
      END IF
C
      END

      !***********************************************************************

      SUBROUTINE REIG(A, E,V, N,NE,EPS, W,NW, N1)
C
      IMPLICIT REAL*8 (A-H,O-Z)
      DIMENSION A(N1,N), E(N), V(N1,N)
      DIMENSION W(N1,6), NW(N)
cAV
      character :: JOBZ = 'V' ! Compute eigenvalues and eigenvectors.
      character :: UPLO = 'U' ! Upper triangles of A and B are stored
      integer :: error, INFO
      integer, save :: LWORK
      real(8), allocatable, save :: W2(:), WORK2(:)
cendAV
c      write(*,*) ' stop :: call to REIG'
C.... SOLVE EIGEN EQUATION AV = EV
      IF((N.LE.1).OR.(NE.LE.0).OR.(N.GT.N1).OR.(NE.GT.N)) THEN
         WRITE(6,*) 'ERROR(REIG) - INPUT: N,NE,N1',N,NE,N1
         RETURN
      ENDIF

      goto 500

      if( .not. allocated(W2) ) then
        LWORK = 3 * N - 1 
        allocate( W2(N), WORK2(LWORK), stat = error )
        if( error > 0 ) stop 'GEIG :: memory allocation error'
        W2 = 0.d0;  WORK2 = 0.d0
      endif

      INFO = 0;  W2 = 0.d0;  WORK2 = 0.d0
      
      call DSYEV( JOBZ, UPLO, N, A, N1, W2, WORK2, LWORK, INFO )
      
c      write(*,*) ' DSYEV returned INFO = ',INFO
      if( INFO / = 0 ) then
        write(*,*) ' DSYEV returned INFO = ',INFO
        stop
      endif
c      write(*,*) '   eigenvalues :'
c      write(*,'(1p,100e16.6)')  W2(1:N)
      ! copy eigenvalues from W2 to E
      E(1:N) = W2(1:N)
      ! copy eigenvectors from A to V
      V(1:N1,1:N) = A(1:N1,1:N)
c      write(*,*) '   eigenvectors :'
c      do i = 1, N1
c         write(*,'(1p,100e16.6)')  A(i,1:N)
c      enddo
      return
c
500   continue
cendAV      
      IF( EPS < 0.0D0 ) EPS = 1.0D-16
C
      IF(N.NE.2) THEN
C
C.... REDUCE TO TRIDIAGONAL FORM BY HOUSEHOLDER'S METHOD
         DO K=1,N-2
            S=0.0D0
            DO I=K+1,N
               S=S+A(I,K)*A(I,K)
            END DO
            W(K,1)=0.0D0
            IF(S.NE.0.0D0) THEN
               SR=DSQRT(S)
               A1=A(K+1,K)
               IF(A1.LT.0.0D0) SR=-SR
               W(K,1)=-SR
               R=1.0D0/(S+A1*SR)
               A(K+1,K)=A1+SR
               DO I=K+1,N
                  S=0.0D0
                  DO J=K+1,I
                     S=S+A(I,J)*A(J,K)
                  END DO
                  DO J=I+1,N
                     S=S+A(J,I)*A(J,K)
                  END DO
                  W(I,1)=S*R
               END DO
               S=0.0D0
               DO I=K+1,N
                  S=S+A(I,K)*W(I,1)
               END DO
               T=S*R*0.5D0
               DO I=K+1,N
                  W(I,1)=W(I,1)-T*A(I,K)
               END DO
               DO J=K+1,N
                  R=W(J,1)
                  S=A(J,K)
                  DO I=J,N
                     A(I,J)=A(I,J)-A(I,K)*R-W(I,1)*S
                  END DO
               END DO
            END IF
         END DO
         W(N-1,1)=A(N,N-1)
C
C.... COMPUTE EIGENVALUES BY BISECTION METHOD
         DO I=1,N
            W(I,6)=A(I,I)
         END DO
         R=DMAX1((DABS(W(1,6))+DABS(W(1,1))),
     $        (DABS(W(N-1,1))+DABS(W(N,6))))
         DO I=2,N-1
            T=DABS(W(I-1,1))+DABS(W(I,6))+DABS(W(I,1))
            IF(T.GT.R) R=T
         END DO
         EPS1=R*1.0D-16
         EPS2=R*EPS
         DO I=1,N-1
            W(I,2)=W(I,1)*W(I,1)
         END DO
         DO I=1,NE
            E(I)=R
         END DO
         F=-R
         DO K=1,NE
            D=E(K)
 100        T=(D+F)*0.5D0
            IF((D-F).GT.EPS2) THEN
               J=0
               I=1
 110           Q=W(I,6)-T
 120           IF(Q.GE.0.0D0) J=J+1
               IF(Q.NE.0.0D0) THEN
                  I=I+1
                  IF(I.LE.N) THEN
                     Q=W(I,6)-T-W(I-1,2)/Q
                     GO TO 120
                  END IF
               ELSE
                  I=I+2
                  IF(I.LE.N) GO TO 110
               END IF
               J=N-J
               IF(J.LT.K) THEN
                  F=T
                  GO TO 100
               END IF
               D=T
               M=MIN0(J,NE)
               DO I=K,M
                  E(I)=T
               END DO
               GO TO 100
            END IF
            E(K)=T
         END DO
      ELSE
         W(1,1)=A(2,1)
         T=(A(1,1)+A(2,2))*0.5D0
         R=A(1,1)*A(2,2)-A(2,1)*A(2,1)
         D=T*T-R
         R=DSQRT(D)
         E(1)=T-R
         IF(NE.EQ.2) E(2)=T+R
         W(1,6)=A(1,1)
         W(2,6)=A(2,2)
         R=DMAX1(DABS(W(1,6)),DABS(W(2,6)))+DABS(W(1,1))
         EPS1=R*1.0D-16
      END IF
C
C.... COMPUTE EIGENVECTORS BY INVERSE ITERATION
      W(N,1)=0.0D0
      MM=584287
      DO I=1,NE
         DO J=1,N
            W(J,2)=W(J,6)-E(I)
            W(J,3)=W(J,1)
            V(J,I)=1.0D0
         END DO
         DO J=1,N-1
            IF(DABS(W(J,2)).GE.DABS(W(J,1))) THEN
               IF(W(J,2).EQ.0.0D0) W(J,2)=1.0D-30
               W(J,5)=W(J,1)/W(J,2)
               NW(J)=0
               W(J+1,2)=W(J+1,2)-W(J,5)*W(J,3)
               W(J,4)=0.0D0
            ELSE
               W(J,5)=W(J,2)/W(J,1)
               NW(J)=1
               W(J,2)=W(J,1)
               T=W(J,3)
               W(J,3)=W(J+1,2)
               W(J,4)=W(J+1,3)
               W(J+1,2)=T-W(J,5)*W(J,3)
               W(J+1,3)=-W(J,5)*W(J,4)
            END IF
         END DO
         IF(W(N,2).EQ.0.0D0) W(N,2)=1.0D-30
         IF(I.NE.1) THEN
            IF(DABS(E(I)-E(I-1)).LT.EPS1) THEN
               DO J=1,N
                  MM=MM*48828125
                  V(J,I)=DBLE(MM)*0.4656613D-9
               END DO
            END IF
         END IF
         T=V(N,I)
         R=V(N-1,I)
         V(N,I)=T/W(N,2)
         V(N-1,I)=(R-W(N-1,3)*V(N,I))/W(N-1,2)
         IF(N.NE.2) THEN
            DO K=N-2,1,-1
               T=V(K,I)
               V(K,I)=(T-W(K,3)*V(K+1,I)-W(K,4)*V(K+2,I))/W(K,2)
            END DO
         END IF
      END DO
C
      IF(N.NE.2) THEN
         DO I=1,N-2
            W(I,1)=-W(I,1)*A(I+1,I)
         END DO
         DO I=1,NE
            DO K=N-2,1,-1
               R=W(K,1)
               IF(R.NE.0.0D0) THEN
                  R=1.0D0/R
                  S=0.0D0
                  DO J=K+1,N
                     S=S+A(J,K)*V(J,I)
                  END DO
                  R=R*S
                  DO J=K+1,N
                     V(J,I)=V(J,I)-R*A(J,K)
                  END DO
               END IF
            END DO
         END DO
      END IF
C
C.... ORTHONORMALIZE EIGENVECTORS
      M=1
      DO I=1,NE
         IF(I.NE.1) THEN
            IF(DABS(E(I)-E(I-1)).LT.EPS1) THEN
               DO J=M,I-1
                  S=0.0D0
                  DO K=1,N
                     S=S+V(K,J)*V(K,I)
                  END DO
                  DO K=1,N
                     V(K,I)=V(K,I)-S*V(K,I)
                  END DO
               END DO
            ELSE
               M=I
            END IF
         END IF
         S=0.0D0
         DO J=1,N
            S=S+V(J,I)*V(J,I)
         END DO
         T=0.0D0
         IF(S.NE.0.0D0) T=DSQRT(1.0D0/S)
         DO J=1,N
            V(J,I)=V(J,I)*T
         END DO
      END DO
C
      RETURN
C
      END

      !***********************************************************************
      !***********************************************************************
      !***********************************************************************
      !***********************************************************************

C     ALGORITHM 602, COLLECTED ALGORITHMS FROM ACM.
C     ALGORITHM APPEARED IN ACM-TRANS. MATH. SOFTWARE, VOL.9, NO. 3,
C     SEP., 1983, P. 355-357.
      SUBROUTINE HURRY(A, L, M, SUM, F, N, ERR, SIGMAS, DA)             
C
C***********************************************************************
C*                                                                     *
C* COMPUTES THE ACCELERATED SUM OF A SERIES OR LIMIT OF A SEQUENCE.    *
C*                                                                     *
C* ARGUMENTS:                                                          *
C*    A      = ARRAY OF ELEMENTS OF SERIES OR SEQUENCE                 *
C*    L      = LEAST NUMBER OF TERMS TO BE USED                        *
C*    M      = MOST NUMBER OF TERMS TO BE USED                         *
C*    SUM    = .TRUE. FOR A SUM, .FALSE. FOR A SEQUENCE                *
C*    F      = FINAL VALUE RETURNED                                    *
C*    N      = NUMBER OF TERMS GIVING 'BEST' RESULT                    *
C*    ERR    = ESTIMATED UNCERTAINTY OF RESULT                         *
C*    SIGMAS = .TRUE. IF UNCERTAINTIES PROVIDED IN DA                  *
C*    DA     = ARRAY OF UNCERTAINTIES OF ELEMENTS                      *
C*                                                                     *
C* INPUTS FROM DRIVER PROGRAM: A, L, M, SUM, SIGMAS, DA                *
C*                                                                     *
C* OUTPUTS TO DRIVER PROGRAM: F, N, ERR                                *
C*                                                                     *
C***********************************************************************
C
C
      LOGICAL SUM, SIGMAS, BETTER, BEGUN, BEFORE, LARGE
      INTEGER I, J, L, M, N
      DOUBLE PRECISION A(M), DA(M), RESULT(50), TRUNC(50), DRDA(50),
     * NOISE(50), W1(50), W2(50), WW1(50,50), WW2(50,50), F, ERR, S,
     * HUGE, SMALL, FACTOR, TEST
C
      DATA HUGE, SMALL, FACTOR /1.0D75,0.01D0,3.0D0/
C
C          HUGE IS USED TO START THE TEST FOR LEAST TRUNCATION
C          ERROR SO FAR.  ANY ALLOWABLE NUMBER LARGER THAN ANY
C          CONCEIVABLE TRUNCATION ERROR WILL DO.  SEE SECTION 2
C          OF ACCOMPANYING ARTICLE FOR EXPLANATION OF SMALL AND
C          FACTOR.
C
      DO 100 I=L,M
C        (COMPUTE RESULT OF ACCELERATING I TERMS)
        IF (I.EQ.L) CALL WHIZ(A, I, W1, W2, SUM, RESULT(I), S, SIGMAS,
     *   DRDA, WW1, WW2, 50)
        IF (I.NE.L) CALL WHIZ1(A, I, W1, W2, SUM, RESULT(I), S, SIGMAS,
     *   DRDA, WW1, WW2, 50)
        NOISE(I) = 0.0D0
C
        IF (.NOT.SIGMAS) GO TO 20
        DO 10 J=1,I
C
C                        ***  WARNING  ***
C              THE NEXT EXECUTABLE STATEMENT CAN AND DOES CAUSE
C              UNDERFLOWS -- THE APPROPRIATE FIX IS MACHINE
C              DEPENDENT.  IF THE MACHINE SETS UNDERFLOWED
C              QUANTITY TO ZERO, NO HARM RESULTS.
C
          NOISE(I) = NOISE(I) + (DA(J)*DRDA(J))**2
   10   CONTINUE
        IF (NOISE(I).GT.0.0) NOISE(I) = DSQRT(NOISE(I))
C
   20   CONTINUE
C              (CHECK TRUNCATION ERROR AND CONVERGENCE)
        IF (I.LE.L) GO TO 30
        TRUNC(I) = DABS(RESULT(I)-RESULT(I-1))
        BETTER = (TRUNC(I).LT.TRUNC(I-1)) .OR. (TRUNC(I).LT.SMALL*
     *   DABS(RESULT(I)))
        BEGUN = BEGUN .OR. (BETTER .AND. BEFORE)
        GO TO 40
   30   CONTINUE
        TRUNC(I) = 0.0D0
        BETTER = .FALSE.
        BEGUN = .FALSE.
        TEST = HUGE
   40   CONTINUE
        BEFORE = BETTER
C
        IF (BEGUN) GO TO 50
        N = I
        GO TO 90
   50   CONTINUE
C              (TEST NUMBER OF TERMS GIVING BEST RESULT SO FAR)
        IF (TRUNC(I).GE.TEST) GO TO 60
        N = I
        TEST = TRUNC(I)
        GO TO 70
   60   CONTINUE
        IF (N.NE.I-1) TEST = (TEST+TRUNC(I))/2.0D0
   70   CONTINUE
        IF (I.EQ.N) GO TO 80
C                 (IS NOISE FROM FROM TERMS LARGE YET?)
        LARGE = TEST.LT.FACTOR*NOISE(I)
        IF (LARGE) GO TO 110
   80   CONTINUE
   90   CONTINUE
  100 CONTINUE
C
  110 CONTINUE
      F = RESULT(N)
      ERR = DMAX1(TRUNC(N),NOISE(N))
C
      RETURN
      END
      SUBROUTINE WHIZ(A, N, QNUM, QDEN, SUM, VALUE, S, DERIVS, DVDA,    
     * DQNUM, DQDEN, M)
C
C***********************************************************************
C*                                                                     *
C* THE U ALGORITHM FOR ACCELERATING A SERIES OR A LIMIT OF A SEQUENCE. *
C*                                                                     *
C* ARGUMENTS:                                                          *
C*    A      = ARRAY OF ELEMENTS OF SERIES OR SEQUENCE                 *
C*    N      = NUMBER OF ELEMENTS IN A                                 *
C*    QNUM   = BACKWARD DIAGONAL OF NUMERATOR ARRAY, AT LEAST N LONG   *
C*    QDEN   = BACKWARD DIAGONAL OF DENOMINATOR ARRAY, AT LEAST N LONG *
C*    SUM    = .TRUE. FOR A SUM, .FALSE. FOR A SEQUENCE                *
C*    VALUE  = ACCELERATED VALUE OF A SUM OR LIMIT                     *
C*    S      = PARTIAL SUM OF SERIES                                   *
C*    DERIVS = .TRUE. IF DERIVATIVES ARE TO BE CALCULATED              *
C*    DVDA   = ARRAY OF CALCULATED DERIVATIVES, D(VALUE)/DA            *
C*    DQNUM  = WORKING STORAGE ARRAY, N*N LONG                         *
C*             (USED ONLY IF DERIVS = .TRUE.)                          *
C*    DQDEN  = WORKING STORAGE ARRAY, N*N LONG                         *
C*             (USED ONLY IF DERIVS = .TRUE.)                          *
C*    M      = FIRST DIMENSION OF DQNUM AND DQDEN ARRAYS               *
C*             (DQNUM AND DQDEN ARE DIMENSIONED (M,N), AND M MUST BE   *
C*              AT LEAST AS LARGE AS THE LARGEST N TO BE USED)         *
C*                                                                     *
C* INPUTS FROM DRIVER, PASSED BY HURRY:  A, N, SUM, DERIVS             *
C*                                                                     *
C* INPUT FROM HURRY:  M                                                *
C*                                                                     *
C* OUTPUTS PASSED TO WHIZ1 BY HURRY:  QNUM, QDEN, VALUE, S, DVDA,      *
C*                                    DQNUM, DQDEN                     *
C*                                                                     *
C***********************************************************************
C
      LOGICAL SUM, DERIVS
      INTEGER N, M, NEXT, I, L
      DOUBLE PRECISION A(N), QNUM(N), QDEN(N), VALUE, S, DVDA(N),
     * DQNUM(M,N), DQDEN(M,N), TERM, FNEXT, FL, RATIO, FJ, FACTOR, C,
     * DTERM, DS
C
      S = 0.0D0
C
      DO 130 NEXT=1,N
C           (GET NEXT DIAGONAL)
        IF (SUM) GO TO 10
        TERM = A(NEXT) - S
        S = A(NEXT)
        GO TO 20
   10   CONTINUE
        TERM = A(NEXT)
        S = A(NEXT) + S
   20   CONTINUE
        L = NEXT - 1
        FNEXT = FLOAT(NEXT)
        QDEN(NEXT) = 1.0D0/(TERM*FNEXT**2)
        QNUM(NEXT) = S*QDEN(NEXT)
        IF (.NOT.DERIVS) GO TO 80
        DO 70 I=1,NEXT
          IF (I.NE.NEXT) GO TO 30
          DTERM = 1.0D0
          DS = 1.0D0
          GO TO 60
   30     CONTINUE
          IF (SUM) GO TO 40
          IF (I.EQ.L) DTERM = -1.0D0
          IF (I.NE.L) DTERM = 0.0D0
          DS = 0.0D0
          GO TO 50
   40     CONTINUE
          DTERM = 0.0D0
          DS = 1.0D0
   50     CONTINUE
   60     CONTINUE
          DQDEN(I,NEXT) = -QDEN(I)*DTERM/TERM
          DQNUM(I,NEXT) = DQDEN(I,NEXT)*S + QDEN(NEXT)*DS
   70   CONTINUE
   80   CONTINUE
        IF (NEXT.LE.1) GO TO 120
        FACTOR = 1.0D0
        FL = FLOAT(L)
        RATIO = FL/FNEXT
        LPLUS1 = L + 1
        DO 110 K=1,L
          J = LPLUS1 - K
          FJ = FLOAT(J)
          C = FACTOR*FJ/FNEXT
          FACTOR = FACTOR*RATIO
          QDEN(J) = QDEN(J+1) - C*QDEN(J)
          QNUM(J) = QNUM(J+1) - C*QNUM(J)
          IF (.NOT.DERIVS) GO TO 100
          DO 90 I=1,L
            DQDEN(I,J) = DQDEN(I,J+1) - C*DQDEN(I,J)
            DQNUM(I,J) = DQNUM(I,J+1) - C*DQNUM(I,J)
   90     CONTINUE
          DQDEN(NEXT,J) = DQDEN(NEXT,J+1)
          DQNUM(NEXT,J) = DQNUM(NEXT,J+1)
  100     CONTINUE
  110   CONTINUE
  120   CONTINUE
  130 CONTINUE
C
      VALUE = QNUM(1)/QDEN(1)
      IF (.NOT.DERIVS) GO TO 150
      DO 140 I=1,N
        DVDA(I) = (DQNUM(I,1)-VALUE*DQDEN(I,1))/QDEN(1)
  140 CONTINUE
  150 CONTINUE
C
      RETURN
      END
      SUBROUTINE WHIZ1(A, N, QNUM, QDEN, SUM, VALUE, S, DERIVS, DVDA,   
     * DQNUM, DQDEN, M)
C
C***********************************************************************
C*                                                                     *
C* THE U ALGORITHM FOR ACCELERATING A SERIES OR A LIMIT OF A SEQUENCE. *
C* THIS SUBROUTINE IS USED TO GET THE N-TERMS RESULT FROM THE          *
C* RESULT OF N-1 TERMS.  WHIZ1 DIFFERS FROM WHIZ IN THAT:              *
C*  (1) THE ALGORITHM IS RUN FOR NEXT=N RATHER THAN FOR NEXT=1 TO N    *
C*  (2) S IS NOT ZEROED AT THE START OF THE SUBROUTINE                 *
C*                                                                     *
C* ARGUMENTS:                                                          *
C*    A      = ARRAY OF ELEMENTS OF SERIES OR SEQUENCE                 *
C*    N      = NUMBER OF ELEMENTS IN A                                 *
C*    QNUM   = BACKWARD DIAGONAL OF NUMERATOR ARRAY, AT LEAST N LONG   *
C*    QDEN   = BACKWARD DIAGONAL OF DENOMINATOR ARRAY, AT LEAST N LONG *
C*    SUM    = .TRUE. FOR A SUM, .FALSE. FOR A LIMIT                   *
C*    VALUE  = ACCELERATED VALUE OF A SUM OR LIMIT                     *
C*    S      = SIMPLE SUM OF SERIES                                    *
C*    DERIVS = .TRUE. IF DERIVATIVES ARE TO BE CALCULATED              *
C*    DVDA   = ARRAY OF CALCULATED DERIVATIVES, D(VALUE)/DA            *
C*    DQNUM  = WORKING STORAGE ARRAY, N*N LONG                         *
C*             (USED ONLY IF DERIVS = .TRUE.)                          *
C*    DQDEN  = WORKING STORAGE ARRAY, N*N LONG                         *
C*             (USED ONLY IF DERIVS = .TRUE.)                          *
C*    M      = FIRST DIMENSION OF DQNUM AND DQDEN ARRAYS               *
C*             (DQNUM AND DQDEN ARE DIMENSIONED (M,N), AND M MUST BE   *
C*              AT LEAST AS LARGE AS THE LARGEST N TO BE USED)         *
C*                                                                     *
C* INPUTS FROM DRIVER, PASSED BY HURRY:  A, N, SUM, DERIVS             *
C*                                                                     *
C* INPUT FROM HURRY:  M                                                *
C*                                                                     *
C* INPUTS FROM WHIZ, PASSED BY HURRY:  QNUM, QDEN, VALUE, S, DVDA,     *
C*                                     DQNUM, DQDEN                    *
C*                                                                     *
C* OUTPUTS TO HURRY:  VALUE, DVDA                                      *
C*                                                                     *
C***********************************************************************
C
      LOGICAL SUM, DERIVS
      INTEGER N, M, NEXT, I, L
      DOUBLE PRECISION A(N), QNUM(N), QDEN(N), VALUE, S, DVDA(N),
     * DQNUM(M,N), DQDEN(M,N), TERM, FNEXT, FL, RATIO, FJ, FACTOR, C,
     * DTERM, DS
C
      NEXT = N
      IF (SUM) GO TO 10
      TERM = A(NEXT) - S
      S = A(NEXT)
      GO TO 20
   10 CONTINUE
      TERM = A(NEXT)
      S = A(NEXT) + S
   20 CONTINUE
      L = NEXT - 1
      FNEXT = FLOAT(NEXT)
      QDEN(NEXT) = 1.0/(TERM*FNEXT**2)
      QNUM(NEXT) = S*QDEN(NEXT)
      IF (.NOT.DERIVS) GO TO 80
      DO 70 I=1,NEXT
        IF (I.NE.NEXT) GO TO 30
        DTERM = 1.0D0
        DS = 1.0D0
        GO TO 60
   30   CONTINUE
        IF (SUM) GO TO 40
        IF (I.EQ.L) DTERM = -1.0D0
        IF (I.NE.L) DTERM = 0.0D0
        DS = 0.0D0
        GO TO 50
   40   CONTINUE
        DTERM = 0.0D0
        DS = 1.0D0
   50   CONTINUE
   60   CONTINUE
        DQDEN(I,NEXT) = -QDEN(I)*DTERM/TERM
        DQNUM(I,NEXT) = DQDEN(I,NEXT)*S + QDEN(NEXT)*DS
   70 CONTINUE
   80 CONTINUE
      IF (NEXT.LE.1) GO TO 120
      FACTOR = 1.0D0
      FL = FLOAT(L)
      RATIO = FL/FNEXT
      LPLUS1 = L + 1
      DO 110 K=1,L
        J = LPLUS1 - K
        FJ = FLOAT(J)
        C = FACTOR*FJ/FNEXT
        FACTOR = FACTOR*RATIO
        QDEN(J) = QDEN(J+1) - C*QDEN(J)
        QNUM(J) = QNUM(J+1) - C*QNUM(J)
        IF (.NOT.DERIVS) GO TO 100
        DO 90 I=1,L
          DQDEN(I,J) = DQDEN(I,J+1) - C*DQDEN(I,J)
          DQNUM(I,J) = DQNUM(I,J+1) - C*DQNUM(I,J)
   90   CONTINUE
        DQDEN(NEXT,J) = DQDEN(NEXT,J+1)
        DQNUM(NEXT,J) = DQNUM(NEXT,J+1)
  100   CONTINUE
  110 CONTINUE
  120 CONTINUE
C
      VALUE = QNUM(1)/QDEN(1)
      IF (.NOT.DERIVS) GO TO 140
      DO 130 I=1,N
        DVDA(I) = (DQNUM(I,1)-VALUE*DQDEN(I,1))/QDEN(1)
  130 CONTINUE
  140 CONTINUE
C
      RETURN
      END


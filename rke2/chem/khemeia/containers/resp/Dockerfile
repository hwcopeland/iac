# RESP/CHELPG charge worker — PySCF HF/6-31G* + antechamber GAFF2 atom types.
# Uses pure-pip Python packages; no conda or proprietary QM code required.

FROM zot.hwcopeland.net/library/ubuntu:22.04

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y --no-install-recommends \
    python3-pip \
    openbabel \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /scripts
COPY requirements.txt ./
# pip: pyscf (HF/CHELPG), scipy (charge fitting), rdkit (conformers),
#      acpype (GAFF2 atom types + antechamber), boto3, mysql-connector.
# Register acpype's bundled AmberTools libs so tleap/antechamber find them.
RUN pip3 install --no-cache-dir -r requirements.txt \
    && python3 -c "import acpype, os; p=os.path.join(os.path.dirname(acpype.__file__),'amber_linux','lib'); open('/etc/ld.so.conf.d/acpype-amber.conf','w').write(p+'\n')" \
    && ldconfig

COPY scripts/ /scripts/

RUN mkdir -p /scratch
WORKDIR /scratch

CMD ["python3", "/scripts/resp_charges.py"]

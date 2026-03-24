# Use the JupyterHub singleuser image as the base
FROM jupyterhub/singleuser:latest

# Switch to root user to install system packages
USER root

# Add Conda to the PATH environment variable
ENV PATH="/opt/conda/bin:$PATH"

# Install Miniconda
RUN wget -qO- https://repo.anaconda.com/miniconda/Miniconda3-latest-Linux-x86_64.sh | bash -b -p /opt/conda && \
    /opt/conda/bin/conda init

# Ensure Conda is properly sourced in all shell sessions
RUN echo ". /opt/conda/etc/profile.d/conda.sh" >> /etc/profile.d/conda.sh && \
    echo "conda activate base" >> /etc/profile.d/conda.sh

# Install additional system dependencies
RUN apt-get update --yes && \
    apt-get install --yes --no-install-recommends \
    openbabel \
    libopenbabel-dev \
    libxrender1 \
    build-essential \
    libboost-all-dev \
    cmake \
    swig && \
    ln -s /usr/bin/python3 /usr/bin/python && \
    apt-get clean && rm -rf /var/lib/apt/lists/*

# Switch back to the non-root user for security
USER ${NB_UID}

# Install Python packages with pip
RUN pip install \
    py3Dmol \
    rdkit \
    nglview \
    prolif \
    scipy \
    pandas \
    numba \
    ipywidgets \
    biopython \
    meeko

# Set environment variable for C++ flags
ENV CXXFLAGS="-DBOOST_TIMER_ENABLE_DEPRECATED"

# Install AutoDock Vina
RUN pip install vina

# Expose JupyterHub singleuser functionality using the new singleuser script
CMD ["start-singleuser.py"]

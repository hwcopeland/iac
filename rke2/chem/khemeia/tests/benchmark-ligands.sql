-- benchmark-ligands.sql
-- HIV-1 Protease (PDB: 1HSG) benchmark ligand set
--
-- 8 FDA-approved HIV protease inhibitors for validating AutoDock Vina docking
-- against a well-characterized target. Expected Vina binding affinities for
-- these inhibitors against 1HSG are in the -7 to -12 kcal/mol range.
--
-- PDBQT is left NULL so prep_ligands.py generates it from SMILES via RDKit +
-- prepare_ligand4. The compound_id uses DrugBank IDs for traceability.
--
-- Usage:
--   mysql -h $MYSQL_HOST -u $MYSQL_USER -p$MYSQL_PASSWORD docking < tests/benchmark-ligands.sql
--   OR via API: POST /api/v1/ligands with the equivalent JSON array

CREATE TABLE IF NOT EXISTS ligands (
    id            INT AUTO_INCREMENT PRIMARY KEY,
    compound_id   VARCHAR(255) NOT NULL,
    smiles        TEXT         NOT NULL,
    pdbqt         MEDIUMBLOB   NULL,
    source_db     VARCHAR(255) NOT NULL,
    created_at    TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_source_db (source_db),
    UNIQUE INDEX idx_compound_source (compound_id, source_db)
);

-- Clear any previous benchmark data for idempotent re-runs
DELETE FROM ligands WHERE source_db = 'hiv-benchmark';

INSERT INTO ligands (compound_id, smiles, pdbqt, source_db) VALUES
    -- Indinavir (Crixivan) -- native ligand MK1 in 1HSG crystal structure
    -- DrugBank: DB00224 | MW: 613.8 | Ki: 0.56 nM
    ('DB00224-indinavir',
     'CC(C)(C)NC(=O)[C@@H]1CN(CCN1C[C@@H](O)C[C@@H](CC2=CC=CC=C2)C(=O)N[C@H]3[C@H](O)CC4=CC=CC=C34)CC5=CC=CN=C5',
     NULL, 'hiv-benchmark'),

    -- Ritonavir (Norvir)
    -- DrugBank: DB00503 | MW: 720.9 | often used as pharmacokinetic booster
    ('DB00503-ritonavir',
     'CC(C)[C@H](NC(=O)N(C)CC1=CSC(=N1)C(C)C)C(=O)N[C@@H](C[C@@H](O)[C@H](CC2=CC=CC=C2)NC(=O)OCC3=CN=CS3)CC4=CC=CC=C4',
     NULL, 'hiv-benchmark'),

    -- Saquinavir (Invirase)
    -- DrugBank: DB01232 | MW: 670.8 | first HIV protease inhibitor approved (1995)
    ('DB01232-saquinavir',
     'CC(C)(C)NC(=O)[C@@H]1CC2=CC=CC=C2C[C@@H]1NC(=O)[C@H](CC3=CC=CC=C3)NC(=O)C4=NC5=CC=CC=C5C=C4',
     NULL, 'hiv-benchmark'),

    -- Nelfinavir (Viracept)
    -- DrugBank: DB00220 | MW: 567.8
    ('DB00220-nelfinavir',
     'OC1CC2=CC=CC=C2[C@@H]1NC(=O)[C@H](CC3=CC=CC=C3)CNC(=O)C4=CC5=C(OC(C)(C)C5)C=C4SC',
     NULL, 'hiv-benchmark'),

    -- Amprenavir (Agenerase)
    -- DrugBank: DB00701 | MW: 505.6
    ('DB00701-amprenavir',
     'CC(C)CN(CC(O)[C@H](CC1=CC=CC=C1)NC(=O)OC2COC3CCCC23)S(=O)(=O)C4=CC=C(N)C=C4',
     NULL, 'hiv-benchmark'),

    -- Lopinavir (Kaletra, combined with ritonavir)
    -- DrugBank: DB01601 | MW: 628.8
    ('DB01601-lopinavir',
     'CC(C)[C@H](NC(=O)[C@H](CC1=CC=CC=C1)CC(=O)N[C@@H](C[C@@H](O)[C@H](CC2=CC=CC=C2)NC(=O)COC3=CC=CC=C3)CC4=CC=CC=C4)C(C)C',
     NULL, 'hiv-benchmark'),

    -- Atazanavir (Reyataz)
    -- DrugBank: DB01072 | MW: 704.9
    ('DB01072-atazanavir',
     'COC(=O)N[C@H](C(=O)N[C@@H](CC1=CC=CC=C1)[C@@H](O)CN(CC2=CC=C(C=C2)C3=CC=CC=N3)NC(=O)[C@H](NC(=O)OC)C(C)(C)C)C(C)(C)C',
     NULL, 'hiv-benchmark'),

    -- Tipranavir (Aptivus) -- non-peptidomimetic inhibitor
    -- DrugBank: DB00932 | MW: 602.7
    ('DB00932-tipranavir',
     'CCC(CC)OC1=CC(=CC(=C1)NS(=O)(=O)C2=CC=C(C=C2)C3=CC(=NN3CC4CC4)C(F)(F)F)[C@H](CC)O',
     NULL, 'hiv-benchmark');

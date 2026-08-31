(module sndr)
(defcolumns (S1 :binary) (S2 :binary) (A :i16))
(defsend s1 bus1 S1 (A))
(defsend s2 bus2 S2 (A))

(module rcvr)
(defcolumns (R1 :binary) (R2 :binary) (X :i16))
(defrecv r1 bus1 R1 (X))
(defrecv r2 bus2 R2 (X))

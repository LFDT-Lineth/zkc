(module sndr)
(defcolumns (SEL :binary) (A :i16) (B :i16))
(defsend s1 bus SEL (A B))

(module rcvr)
(defcolumns (SEL :binary) (X :i16) (Y :i16))
(defrecv r1 bus SEL (X Y))

(module s1)
(defcolumns (SEL :binary) (A :i16) (B :i16))
(defsend snd1 bus SEL (A B))

(module s2)
(defcolumns (SEL :binary) (A :i16) (B :i16))
(defsend snd2 bus SEL (A B))

(module r1)
(defcolumns (SEL :binary) (X :i16) (Y :i16))
(defrecv rcv1 bus SEL (X Y))

(module r2)
(defcolumns (SEL :binary) (X :i16) (Y :i16))
(defrecv rcv2 bus SEL (X Y))

;;error:4:10-12:duplicate handle
(defcolumns (SEL :binary) (A :i16) (X :i16))
(defsend s1 bus SEL (A))
(defrecv s1 bus SEL (X))

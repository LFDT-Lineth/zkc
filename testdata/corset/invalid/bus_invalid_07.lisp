;;error:7:17-23:conflicting context
(module m1)
(defcolumns (SEL :binary) (X :i16))

(module m2)
(defcolumns (A :i16))
(defsend s1 bus m1.SEL (A))

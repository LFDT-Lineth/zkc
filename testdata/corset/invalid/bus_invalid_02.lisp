;;error:3:17-20:non-binary selector encountered
(defcolumns (SEL :i16) (A :i16))
(defsend s1 bus SEL (A))
